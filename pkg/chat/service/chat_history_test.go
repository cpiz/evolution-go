package chat_service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestMapHistorySyncMessages(t *testing.T) {
	const group = "120363000000000000@g.us"
	const other = "120363000000000001@g.us"
	chat, err := types.ParseJID(group)
	if err != nil {
		t.Fatal(err)
	}
	since := time.Unix(100, 0)
	until := time.Unix(200, 0)
	item := func(id, jid string, timestamp uint64) *waHistorySync.HistorySyncMsg {
		return &waHistorySync.HistorySyncMsg{Message: &waWeb.WebMessageInfo{
			Key: &waCommon.MessageKey{
				ID: proto.String(id), RemoteJID: proto.String(jid),
				Participant: proto.String("628123456789@s.whatsapp.net"),
			},
			Message:          &waE2E.Message{Conversation: proto.String("test history")},
			MessageTimestamp: proto.Uint64(timestamp),
		}}
	}
	history := &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{
		{ID: proto.String(other), Messages: []*waHistorySync.HistorySyncMsg{item("other", other, 150)}},
		{ID: proto.String(group), Messages: []*waHistorySync.HistorySyncMsg{
			item("mismatched", other, 150), item("too-old", group, 99),
			item("valid", group, 100), item("anchor", group, 200),
		}},
	}}
	result := mapHistorySyncMessages(&whatsmeow.Client{}, history, &HistorySyncRequestStruct{
		MessageInfo: &types.MessageInfo{MessageSource: types.MessageSource{Chat: chat}},
		Count:       50,
		Since:       &since,
		Until:       &until,
	})
	if len(result) != 1 || result[0].Info.ID != "valid" || !result[0].Info.Timestamp.Equal(since) {
		t.Fatalf("unexpected filtered history: %+v", result)
	}
	payload, err := json.Marshal(result[0])
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Info struct {
			Chat string
		}
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Info.Chat != group {
		t.Fatalf("unexpected archive payload: %s (%v)", payload, err)
	}
}

type testHistoryClient struct {
	whatsmeow.Client
	handler whatsmeow.EventHandler
	send    func(context.Context, types.JID, whatsmeow.SendRequestExtra) error
}

func (c *testHistoryClient) GenerateMessageID() types.MessageID { return "peer-request-id" }
func (c *testHistoryClient) AddEventHandler(handler whatsmeow.EventHandler) uint32 {
	c.handler = handler
	return 1
}
func (c *testHistoryClient) SendMessage(ctx context.Context, jid types.JID, _ *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	return whatsmeow.SendResponse{}, c.send(ctx, jid, extra[0])
}

func TestHistoryRequestTracksLateResultAndReusesIt(t *testing.T) {
	for _, confirm := range []bool{true, false} {
		name := "send_confirmation_failed"
		if confirm {
			name = "result_wait_failed"
		}
		t.Run(name, func(t *testing.T) {
			logDir := t.TempDir()
			service := &chatService{loggerWrapper: logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: logDir})}
			chat := types.NewJID("120363000000000000", types.GroupServer)
			own := types.NewJID("628111111111", types.DefaultUserServer)
			since, until := time.Unix(100, 0), time.Unix(200, 0)
			scope := &HistorySyncRequestStruct{
				MessageInfo: &types.MessageInfo{MessageSource: types.MessageSource{Chat: chat}, ID: "anchor", Timestamp: until},
				Count:       50, Since: &since, Until: &until,
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sends := 0
			client := &testHistoryClient{send: func(sendCtx context.Context, jid types.JID, extra whatsmeow.SendRequestExtra) error {
				sends++
				deadline, ok := sendCtx.Deadline()
				if !ok || time.Until(deadline) > historySendTimeout || jid != own || extra.ID != "peer-request-id" || !extra.Peer {
					t.Fatal("request did not use the peer destination, explicit ID and send deadline")
				}
				if !confirm {
					return context.DeadlineExceeded
				}
				cancel()
				return nil
			}}
			_, err := service.requestHistory(ctx, client, "instance", own, scope, true)
			if err == nil || !strings.Contains(err.Error(), strings.ReplaceAll(name, "_", " ")) ||
				(!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
				t.Fatalf("unexpected request error: %v", err)
			}

			client.handler(&events.HistorySync{Data: &waHistorySync.HistorySync{
				SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
				Conversations: []*waHistorySync.Conversation{{ID: proto.String("other@g.us")}},
			}})
			otherScope := *scope
			otherScope.Count = 25
			if _, err := service.requestHistory(context.Background(), client, "instance", own, &otherScope, true); err == nil {
				t.Fatal("a competing request for the same group was accepted")
			}
			pending, _ := service.historyRequests.Load(historyKey{instanceID: "instance", groupJID: chat})
			select {
			case <-pending.(*pendingHistory).ready:
				t.Fatal("unrelated history was linked to the request")
			default:
			}
			client.handler(&events.HistorySync{Data: &waHistorySync.HistorySync{
				SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
				Conversations: []*waHistorySync.Conversation{{ID: proto.String(chat.String())}},
			}})
			result, err := service.requestHistory(context.Background(), client, "instance", own, scope, true)
			if err != nil || len(result.([]historySyncMessage)) != 0 || sends != 1 {
				t.Fatalf("late result was not reused: result=%v err=%v sends=%d", result, err, sends)
			}
			logs, err := os.ReadFile(filepath.Join(logDir, "instance", "instance.log"))
			if err != nil || !strings.Contains(string(logs), "request_id=peer-request-id group_jid="+chat.String()) ||
				!strings.Contains(string(logs), "returned_count=0 in_scope_count=0 late=true") ||
				strings.Contains(string(logs), "history_send_confirmed") != confirm {
				t.Fatalf("missing stage or late-result evidence: %s (%v)", logs, err)
			}
		})
	}
}
