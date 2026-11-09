package chat_service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	historySendTimeout = 8 * time.Second
	historyWaitTimeout = 25 * time.Second
	historyRetention   = 20 * time.Minute
)

type historyKey struct {
	instanceID string
	groupJID   types.JID
}

type historyMessageParser interface {
	ParseWebMessage(types.JID, *waWeb.WebMessageInfo) (*events.Message, error)
}

type historyClient interface {
	historyMessageParser
	GenerateMessageID() types.MessageID
	BuildHistorySyncRequest(*types.MessageInfo, int) *waE2E.Message
	AddEventHandler(whatsmeow.EventHandler) uint32
	RemoveEventHandler(uint32) bool
	SendMessage(context.Context, types.JID, *waE2E.Message, ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
}

type pendingHistory struct {
	id       types.MessageID
	scope    *HistorySyncRequestStruct
	ready    chan struct{}
	done     sync.Once
	late     atomic.Bool
	messages []historySyncMessage
}

func (p *pendingHistory) matches(scope *HistorySyncRequestStruct) bool {
	return p.scope.Since != nil && p.scope.Until != nil && scope.Since != nil && scope.Until != nil &&
		p.scope.Count == scope.Count && p.scope.Since.Equal(*scope.Since) && p.scope.Until.Equal(*scope.Until) &&
		p.scope.MessageInfo.ID == scope.MessageInfo.ID &&
		p.scope.MessageInfo.IsFromMe == scope.MessageInfo.IsFromMe &&
		p.scope.MessageInfo.Timestamp.Equal(scope.MessageInfo.Timestamp)
}

func (p *pendingHistory) await(ctx context.Context) ([]historySyncMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, historyWaitTimeout)
	defer cancel()
	select {
	case <-p.ready:
		return p.messages, nil
	default:
	}
	select {
	case <-p.ready:
		return p.messages, nil
	case <-ctx.Done():
		p.late.Store(true)
		return nil, fmt.Errorf("history result wait failed request_id=%s: %w", p.id, ctx.Err())
	}
}

func (c *chatService) requestHistory(ctx context.Context, client historyClient, instanceID string, ownJID types.JID, data *HistorySyncRequestStruct, wait bool) (any, error) {
	chat := data.MessageInfo.Chat
	key := historyKey{instanceID: instanceID, groupJID: chat}
	pending := &pendingHistory{id: client.GenerateMessageID(), scope: data, ready: make(chan struct{})}
	active, exists := c.historyRequests.LoadOrStore(key, pending)
	if exists {
		previous := active.(*pendingHistory)
		if wait && previous.matches(data) {
			// A retry for the same scope waits on the original request or reuses its result.
			return previous.await(ctx)
		}
		return nil, fmt.Errorf("history request already tracked for this chat request_id=%s", previous.id)
	}

	log := c.loggerWrapper.GetLogger(instanceID)
	started := time.Now()
	log.LogInfo("history_request_started request_id=%s group_jid=%s requested_count=%d", pending.id, chat, data.Count)
	if wait {
		handlerID := client.AddEventHandler(func(raw any) {
			event, ok := raw.(*events.HistorySync)
			if !ok || event == nil || !hasHistoryConversation(event.Data, chat) {
				return
			}
			count := 0
			for _, conversation := range event.Data.GetConversations() {
				if conversation.GetID() == chat.String() {
					count += len(conversation.GetMessages())
				}
			}
			messages := mapHistorySyncMessages(client, event.Data, data)
			pending.done.Do(func() {
				pending.messages = messages
				close(pending.ready)
			})
			log.LogInfo("history_result_received request_id=%s group_jid=%s returned_count=%d in_scope_count=%d late=%t elapsed_ms=%d correlation=instance_and_chat",
				pending.id, chat, count, len(messages), pending.late.Load(), time.Since(started).Milliseconds())
		})
		// Keep listening beyond the HTTP deadline. A different request for this chat
		// must wait until expiry, since the decoded event does not carry a request ID.
		time.AfterFunc(historyRetention, func() {
			client.RemoveEventHandler(handlerID)
			c.historyRequests.CompareAndDelete(key, pending)
		})
	} else {
		defer c.historyRequests.CompareAndDelete(key, pending)
	}

	peerRequest := client.BuildHistorySyncRequest(data.MessageInfo, data.Count)
	sendCtx, cancelSend := context.WithTimeout(ctx, historySendTimeout)
	ack, err := client.SendMessage(sendCtx, ownJID.ToNonAD(), peerRequest,
		whatsmeow.SendRequestExtra{Peer: true, ID: pending.id})
	cancelSend()
	if err != nil {
		pending.late.Store(true)
		log.LogWarn("history_send_unconfirmed request_id=%s group_jid=%s elapsed_ms=%d", pending.id, chat, time.Since(started).Milliseconds())
		return nil, fmt.Errorf("history send confirmation failed request_id=%s: %w", pending.id, err)
	}
	log.LogInfo("history_send_confirmed request_id=%s group_jid=%s elapsed_ms=%d", pending.id, chat, time.Since(started).Milliseconds())
	if !wait {
		return ack, nil
	}
	messages, err := pending.await(ctx)
	if err != nil {
		log.LogWarn("history_result_wait_ended request_id=%s group_jid=%s elapsed_ms=%d", pending.id, chat, time.Since(started).Milliseconds())
	}
	return messages, err
}

func hasHistoryConversation(data *waHistorySync.HistorySync, chat types.JID) bool {
	if data == nil || data.GetSyncType() != waHistorySync.HistorySync_ON_DEMAND {
		return false
	}
	for _, conversation := range data.GetConversations() {
		if conversation.GetID() == chat.String() {
			return true
		}
	}
	return false
}
