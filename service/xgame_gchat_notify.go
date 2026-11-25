// XGAME: forwards automatic channel/key disable notices (upstream key out of
// balance, revoked, ...) to a Google Chat space through an incoming webhook.
package service

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/bytedance/gopkg/util/gopool"
)

const xgameGoogleChatWebhookEnv = "XGAME_GCHAT_WEBHOOK_URL"

const xgameGoogleChatMaxRunes = 4000

var xgameGoogleChatClient = &http.Client{Timeout: 10 * time.Second}

// XGAME: params carries Name, Id and Reason for the shared disabled-channel message.
func xgameNotifyChannelDisabled(params map[string]any) {
	webhookURL := os.Getenv(xgameGoogleChatWebhookEnv)
	if webhookURL == "" {
		return
	}
	content := i18n.Translate("", i18n.MsgChannelNotifyDisabledContent, params)
	text := fmt.Sprintf("*%s*\n%s", common.SystemName, content)
	gopool.Go(func() {
		if err := sendGoogleChatMessage(webhookURL, text); err != nil {
			common.SysError(common.LogText("failed to send Google Chat channel notice: %s", err.Error()))
		}
	})
}

// sendGoogleChatMessage posts text to a Google Chat incoming webhook. Errors
// never include the webhook URL, whose query string holds the space credentials.
func sendGoogleChatMessage(webhookURL string, text string) error {
	if runes := []rune(text); len(runes) > xgameGoogleChatMaxRunes {
		text = string(runes[:xgameGoogleChatMaxRunes]) + "…"
	}
	body, err := common.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid Google Chat webhook URL")
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := xgameGoogleChatClient.Do(req)
	if err != nil {
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			return urlErr.Err
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Google Chat webhook returned status %d", resp.StatusCode)
	}
	return nil
}
