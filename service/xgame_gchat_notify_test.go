// XGAME: tests for the Google Chat channel-disable notifier.
package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendGoogleChatMessagePostsTextPayload(t *testing.T) {
	var gotBody map[string]string
	var gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, common.Unmarshal(raw, &gotBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := sendGoogleChatMessage(server.URL+"/v1/spaces/x/messages?key=k&token=t", "*XGame*\nChannel \"deepseek\" (#3) has been disabled")

	require.NoError(t, err)
	assert.Equal(t, "application/json; charset=UTF-8", gotContentType)
	assert.Equal(t, map[string]string{"text": "*XGame*\nChannel \"deepseek\" (#3) has been disabled"}, gotBody)
}

func TestSendGoogleChatMessageTruncatesLongText(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, common.Unmarshal(raw, &gotBody))
	}))
	defer server.Close()

	err := sendGoogleChatMessage(server.URL, strings.Repeat("é", xgameGoogleChatMaxRunes+10))

	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("é", xgameGoogleChatMaxRunes)+"…", gotBody["text"])
}

func TestSendGoogleChatMessageErrorsHideWebhookCredentials(t *testing.T) {
	const secret = "super-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	closedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedServer.Close()
	defer server.Close()

	tests := []struct {
		name       string
		webhookURL string
		wantErr    string
	}{
		{name: "non-2xx status", webhookURL: server.URL + "/?token=" + secret, wantErr: "status 403"},
		{name: "unreachable host", webhookURL: closedServer.URL + "/?token=" + secret},
		{name: "malformed url", webhookURL: "http://[::1" + "/?token=" + secret, wantErr: "invalid Google Chat webhook URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sendGoogleChatMessage(tt.webhookURL, "hello")

			require.Error(t, err)
			assert.NotContains(t, err.Error(), secret)
			if tt.wantErr != "" {
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}
