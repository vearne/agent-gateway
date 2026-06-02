package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type API struct {
	httpClient *http.Client
	baseURL    string
	cdnBaseURL string
	token      string
	botAgent   string
}

func NewAPI(baseURL, cdnBaseURL string) *API {
	return &API{
		baseURL:    baseURL,
		cdnBaseURL: cdnBaseURL,
		botAgent:   BotAgent,
		httpClient: &http.Client{Timeout: 40 * time.Second},
	}
}

func (a *API) SetToken(token string) {
	a.token = token
}

func (a *API) SetBaseURL(url string) {
	a.baseURL = url
}

func (a *API) randomWechatUIN() string {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	n := binary.LittleEndian.Uint32(buf[:])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", n)))
}

func (a *API) baseInfo() *BaseInfo {
	return &BaseInfo{
		ChannelVersion: ChannelVersion,
		BotAgent:       a.botAgent,
	}
}

func (a *API) authHeaders() map[string]string {
	return map[string]string{
		"Content-Type":      "application/json",
		"AuthorizationType": "ilink_bot_token",
		"Authorization":     "Bearer " + a.token,
		"X-WECHAT-UIN":      a.randomWechatUIN(),
	}
}

func (a *API) doRequest(ctx context.Context, method, url string, body interface{}, timeout time.Duration) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("weixin api: marshal: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("weixin api: new request: %w", err)
	}

	for k, v := range a.authHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weixin api: do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("weixin api: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weixin api: status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (a *API) get(ctx context.Context, endpoint string, timeout time.Duration) ([]byte, error) {
	url := a.baseURL + "/ilink/bot/" + endpoint
	return a.doRequest(ctx, http.MethodGet, url, nil, timeout)
}

func (a *API) post(ctx context.Context, endpoint string, body interface{}, timeout time.Duration) ([]byte, error) {
	url := a.baseURL + "/ilink/bot/" + endpoint
	return a.doRequest(ctx, http.MethodPost, url, body, timeout)
}

func (a *API) GetBotQRCode(ctx context.Context) (*QRCodeResp, error) {
	data, err := a.post(ctx, "get_bot_qrcode?bot_type="+BotType, map[string]interface{}{}, 10*time.Second)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Ret  int         `json:"ret"`
		Data *QRCodeResp `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("weixin api: decode qrcode: %w", err)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("weixin api: qrcode response empty")
	}
	return resp.Data, nil
}

func (a *API) GetQRCodeStatus(ctx context.Context, qrcode string) (*QRStatusResp, error) {
	data, err := a.get(ctx, "get_qrcode_status?qrcode="+qrcode, 35*time.Second)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Ret  int           `json:"ret"`
		Data *QRStatusResp `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("weixin api: decode qrstatus: %w", err)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("weixin api: qrstatus response empty")
	}
	return resp.Data, nil
}

func (a *API) GetUpdates(ctx context.Context, syncBuf string) (*GetUpdatesResp, error) {
	req := &GetUpdatesReq{
		GetUpdatesBuf: syncBuf,
		BaseInfo:      a.baseInfo(),
	}
	data, err := a.post(ctx, "getupdates", req, 40*time.Second)
	if err != nil {
		return nil, err
	}
	var resp GetUpdatesResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("weixin api: decode getupdates: %w", err)
	}
	if resp.Errcode != 0 {
		zap.L().Warn("weixin getupdates errcode",
			zap.Int("errcode", resp.Errcode),
			zap.String("errmsg", resp.Errmsg))
	}
	return &resp, nil
}

func (a *API) SendMessage(ctx context.Context, msg *WeixinMessage) error {
	req := &SendMessageReq{
		Msg:      msg,
		BaseInfo: a.baseInfo(),
	}
	data, err := a.post(ctx, "sendmessage", req, 10*time.Second)
	if err != nil {
		return err
	}
	var resp struct {
		Ret     int    `json:"ret"`
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("weixin api: decode sendmessage: %w", err)
	}
	if resp.Ret != 0 {
		return fmt.Errorf("weixin api: sendmessage ret=%d errcode=%d msg=%s",
			resp.Ret, resp.Errcode, resp.Errmsg)
	}
	return nil
}

func (a *API) SendTyping(ctx context.Context, userID, typingTicket string) error {
	req := &SendTypingReq{
		ILinkUserID:  userID,
		TypingTicket: typingTicket,
		Status:       1,
		BaseInfo:     a.baseInfo(),
	}
	_, err := a.post(ctx, "sendtyping", req, 5*time.Second)
	return err
}

func (a *API) CancelTyping(ctx context.Context, userID, typingTicket string) error {
	req := &SendTypingReq{
		ILinkUserID:  userID,
		TypingTicket: typingTicket,
		Status:       2,
		BaseInfo:     a.baseInfo(),
	}
	_, err := a.post(ctx, "sendtyping", req, 5*time.Second)
	return err
}

func (a *API) GetConfig(ctx context.Context, userID, contextToken string) (*GetConfigResp, error) {
	req := &GetConfigReq{
		ILinkUserID:  userID,
		ContextToken: contextToken,
		BaseInfo:     a.baseInfo(),
	}
	data, err := a.post(ctx, "getconfig", req, 10*time.Second)
	if err != nil {
		return nil, err
	}
	var resp GetConfigResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("weixin api: decode getconfig: %w", err)
	}
	return &resp, nil
}

func (a *API) GetUploadURL(ctx context.Context, req *GetUploadUrlReq) (*GetUploadUrlResp, error) {
	req.BaseInfo = a.baseInfo()
	data, err := a.post(ctx, "getuploadurl", req, 10*time.Second)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Ret  int              `json:"ret"`
		Data *GetUploadUrlResp `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("weixin api: decode getuploadurl: %w", err)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("weixin api: getuploadurl response empty")
	}
	return resp.Data, nil
}

func (a *API) UploadToCDN(ctx context.Context, uploadURL string, fileData []byte) error {
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPut, uploadURL, bytes.NewReader(fileData))
	if err != nil {
		return fmt.Errorf("weixin cdn: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("weixin cdn: upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("weixin cdn: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (a *API) NotifyStart(ctx context.Context) error {
	req := &NotifyReq{BaseInfo: a.baseInfo()}
	_, err := a.post(ctx, "notifystart", req, 5*time.Second)
	return err
}

func (a *API) NotifyStop(ctx context.Context) error {
	req := &NotifyReq{BaseInfo: a.baseInfo()}
	_, err := a.post(ctx, "notifystop", req, 5*time.Second)
	return err
}
