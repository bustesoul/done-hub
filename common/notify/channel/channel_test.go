package channel_test

import (
	"context"
	"fmt"
	"testing"

	"done-hub/common/notify/channel"
	"done-hub/common/test"

	"github.com/stretchr/testify/assert"
)

func TestDingTalkSend(t *testing.T) {
	cfg := test.RequireExternalIntegration(t, "notify.dingtalk.token", "notify.dingtalk.secret")
	access_token := cfg.GetString("notify.dingtalk.token")
	secret := cfg.GetString("notify.dingtalk.secret")
	dingTalk := channel.NewDingTalk(access_token, secret)

	err := dingTalk.Send(context.Background(), "Test Title", "*Test Message*")
	fmt.Println(err)
	assert.Nil(t, err)
}

func TestDingTalkSendWithKeyWord(t *testing.T) {
	cfg := test.RequireExternalIntegration(t, "notify.dingtalk.token", "notify.dingtalk.keyWord")
	access_token := cfg.GetString("notify.dingtalk.token")
	keyWord := cfg.GetString("notify.dingtalk.keyWord")

	dingTalk := channel.NewDingTalkWithKeyWord(access_token, keyWord)

	err := dingTalk.Send(context.Background(), "Test Title", "Test Message")
	assert.Nil(t, err)
}

func TestDingTalkSendError(t *testing.T) {
	cfg := test.RequireExternalIntegration(t, "notify.dingtalk.token")
	access_token := cfg.GetString("notify.dingtalk.token")
	secret := "test"
	dingTalk := channel.NewDingTalk(access_token, secret)

	err := dingTalk.Send(context.Background(), "Test Title", "*Test Message*")
	fmt.Println(err)
	assert.Error(t, err)
}

func TestLarkSend(t *testing.T) {
	cfg := test.RequireExternalIntegration(t, "notify.lark.token", "notify.lark.secret")
	access_token := cfg.GetString("notify.lark.token")
	secret := cfg.GetString("notify.lark.secret")
	dingTalk := channel.NewLark(access_token, secret)

	err := dingTalk.Send(context.Background(), "Test Title", "*Test Message*")
	fmt.Println(err)
	assert.Nil(t, err)
}

func TestLarkSendWithKeyWord(t *testing.T) {
	cfg := test.RequireExternalIntegration(t, "notify.lark.token", "notify.lark.keyWord")
	access_token := cfg.GetString("notify.lark.token")
	keyWord := cfg.GetString("notify.lark.keyWord")

	dingTalk := channel.NewLarkWithKeyWord(access_token, keyWord)

	err := dingTalk.Send(context.Background(), "Test Title", "Test Message\n\n- 111\n- 222")
	assert.Nil(t, err)
}

func TestLarkSendError(t *testing.T) {
	cfg := test.RequireExternalIntegration(t, "notify.lark.token")
	access_token := cfg.GetString("notify.lark.token")
	secret := "test"
	dingTalk := channel.NewLark(access_token, secret)

	err := dingTalk.Send(context.Background(), "Title", "*Message*")
	fmt.Println(err)
	assert.Error(t, err)
}

func TestPushdeerSend(t *testing.T) {
	cfg := test.RequireExternalIntegration(t, "notify.pushdeer.pushkey")
	pushkey := cfg.GetString("notify.pushdeer.pushkey")
	dingTalk := channel.NewPushdeer(pushkey, "")

	err := dingTalk.Send(context.Background(), "Test Title", "*Test Message*")
	fmt.Println(err)
	assert.Nil(t, err)
}

func TestPushdeerSendError(t *testing.T) {
	test.RequireExternalIntegration(t)
	pushkey := "test"
	dingTalk := channel.NewPushdeer(pushkey, "")

	err := dingTalk.Send(context.Background(), "Test Title", "*Test Message*")
	fmt.Println(err)
	assert.Error(t, err)
}

func TestTelegramSend(t *testing.T) {
	cfg := test.RequireExternalIntegration(t, "notify.telegram.bot_api_key", "notify.telegram.chat_id")
	secret := cfg.GetString("notify.telegram.bot_api_key")
	chatID := cfg.GetString("notify.telegram.chat_id")
	httpProxy := cfg.GetString("notify.telegram.http_proxy")
	dingTalk := channel.NewTelegram(secret, chatID, httpProxy)

	err := dingTalk.Send(context.Background(), "Test Title", "*Test Message*")
	fmt.Println(err)
	assert.Nil(t, err)
}

func TestTelegramSendError(t *testing.T) {
	cfg := test.RequireExternalIntegration(t, "notify.telegram.chat_id")
	secret := "test"
	chatID := cfg.GetString("notify.telegram.chat_id")
	dingTalk := channel.NewTelegram(secret, chatID, "")

	err := dingTalk.Send(context.Background(), "Test Title", "*Test Message*")
	fmt.Println(err)
	assert.Error(t, err)
}
