package chat_service

import (
	"context"
	"errors"
	"sync"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
)

type ChatService interface {
	ChatPin(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatUnpin(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatArchive(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatUnarchive(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatMute(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatUnmute(data *BodyStruct, instance *instance_model.Instance) (string, error)
	HistorySyncRequest(ctx context.Context, data *HistorySyncRequestStruct, instance *instance_model.Instance) (any, error)
}

type chatService struct {
	clientPointer    map[string]*whatsmeow.Client
	whatsmeowService whatsmeow_service.WhatsmeowService
	loggerWrapper    *logger_wrapper.LoggerManager
	historyRequests  sync.Map
}

type BodyStruct struct {
	Chat string `json:"chat"`
}

type HistorySyncRequestStruct struct {
	MessageInfo *types.MessageInfo `json:"messageInfo"`
	Count       int                `json:"count"`
	Since       *time.Time         `json:"since,omitempty"`
	Until       *time.Time         `json:"until,omitempty"`
}

type historySyncMessage struct {
	Info    types.MessageInfo
	Message *waE2E.Message
}

func (c *chatService) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	client := c.clientPointer[instanceId]
	c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := c.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			c.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Instance started, waiting 2 seconds...", instanceId)
		time.Sleep(2 * time.Second)

		client = c.clientPointer[instanceId]
		c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Checking new client - Exists: %v, Connected: %v",
			instanceId,
			client != nil,
			client != nil && client.IsConnected())

		if client == nil || !client.IsConnected() {
			c.loggerWrapper.GetLogger(instanceId).LogError("[%s] New client validation failed - Exists: %v, Connected: %v",
				instanceId,
				client != nil,
				client != nil && client.IsConnected())
			return nil, errors.New("no active session found")
		}
	} else if !client.IsConnected() {
		c.loggerWrapper.GetLogger(instanceId).LogError("[%s] Existing client is disconnected - Connected status: %v",
			instanceId,
			client.IsConnected())
		return nil, errors.New("client disconnected")
	}

	c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Client successfully validated - Connected: %v", instanceId, client.IsConnected())
	return client, nil
}

func (c *chatService) ChatPin(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildPin(recipient, true))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error pin chat: %v", instance.Id, err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatUnpin(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildPin(recipient, false))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unpin chat: %v", instance.Id, err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatArchive(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildArchive(recipient, true, time.Time{}, nil))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error archive chat: %v", instance.Id, err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatUnarchive(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildArchive(recipient, false, time.Time{}, nil))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unarchive chat: %v", instance.Id, err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatMute(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildMute(recipient, true, 1*time.Hour))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error mute chat: %v", instance.Id, err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatUnmute(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildMute(recipient, false, 0*time.Hour))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unmute chat: %v", instance.Id, err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) HistorySyncRequest(ctx context.Context, data *HistorySyncRequestStruct, instance *instance_model.Instance) (any, error) {
	if data == nil || data.MessageInfo == nil || data.MessageInfo.Chat.IsEmpty() ||
		data.MessageInfo.ID == "" || data.MessageInfo.Timestamp.IsZero() || data.Count < 1 {
		return nil, errors.New("a chat, anchor message and positive count are required")
	}
	wait := data.Since != nil || data.Until != nil
	if wait && (data.MessageInfo.Chat.Server != types.GroupServer || data.Count > 50 ||
		data.Since == nil || data.Until == nil || !data.Since.Before(*data.Until) ||
		data.Until.After(data.MessageInfo.Timestamp)) {
		return nil, errors.New("group history requires a range ending at or before the anchor and a count of at most 50")
	}
	client := c.clientPointer[instance.Id]
	if client == nil || !client.IsConnected() || !client.IsLoggedIn() {
		return nil, errors.New("history sync requires an online instance")
	}
	return c.requestHistory(ctx, client, instance.Id, client.Store.GetJID(), data, wait)
}

func mapHistorySyncMessages(client historyMessageParser, history *waHistorySync.HistorySync, data *HistorySyncRequestStruct) []historySyncMessage {
	messages := make([]historySyncMessage, 0)
	chat := data.MessageInfo.Chat
	for _, conversation := range history.GetConversations() {
		if conversation.GetID() != chat.String() {
			continue
		}
		for _, item := range conversation.GetMessages() {
			msg := item.GetMessage()
			key := msg.GetKey()
			timestamp := time.Unix(int64(msg.GetMessageTimestamp()), 0)
			if key.GetRemoteJID() != chat.String() || key.GetID() == "" ||
				key.GetID() == data.MessageInfo.ID || msg.GetMessage() == nil ||
				timestamp.Before(*data.Since) || !timestamp.Before(*data.Until) {
				continue
			}
			event, err := client.ParseWebMessage(chat, msg)
			if err == nil && event.Message != nil {
				messages = append(messages, historySyncMessage{Info: event.Info, Message: event.Message})
				if len(messages) == data.Count {
					return messages
				}
			}
		}
	}
	return messages
}

func NewChatService(
	clientPointer map[string]*whatsmeow.Client,
	whatsmeowService whatsmeow_service.WhatsmeowService,
	loggerWrapper *logger_wrapper.LoggerManager,
) ChatService {
	return &chatService{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
		loggerWrapper:    loggerWrapper,
	}
}
