package weixin

// iLink protocol constants.
const (
	DefaultBaseURL    = "https://ilinkai.weixin.qq.com"
	DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	BotType           = "3"

	MessageStateNew        = 0
	MessageStateGenerating = 1
	MessageStateFinish     = 2

	MessageTypeUser = 1
	MessageTypeBot  = 2

	ItemNone          = 0
	ItemText          = 1
	ItemImage         = 2
	ItemVoice         = 3
	ItemFile          = 4
	ItemVideo         = 5
	ItemToolCallStart = 11
	ItemToolCallResult = 12

	UploadMediaImage = 1
	UploadMediaVideo = 2
	UploadMediaFile  = 3
	UploadMediaVoice = 4

	SessionExpiredErrcode = -14

	ChannelVersion = "1.0.0"
	BotAgent       = "agent-gateway/1.0"
)

// BaseInfo is attached to every API request.
type BaseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
	BotAgent       string `json:"bot_agent,omitempty"`
}

// Message item types.

type TextItem struct {
	Text string `json:"text,omitempty"`
}

type CDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AESKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
	FullURL           string `json:"full_url,omitempty"`
}

type ImageItem struct {
	Media      *CDNMedia `json:"media,omitempty"`
	ThumbMedia *CDNMedia `json:"thumb_media,omitempty"`
	AESKey     string    `json:"aeskey,omitempty"`
}

type VoiceItem struct {
	Media      *CDNMedia `json:"media,omitempty"`
	EncodeType int       `json:"encode_type,omitempty"`
	Playtime   int       `json:"playtime,omitempty"`
	Text       string    `json:"text,omitempty"`
}

type FileItem struct {
	Media    *CDNMedia `json:"media,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	MD5      string    `json:"md5,omitempty"`
	Len      string    `json:"len,omitempty"`
}

type VideoItem struct {
	Media      *CDNMedia `json:"media,omitempty"`
	VideoSize  int       `json:"video_size,omitempty"`
	VideoMD5   string    `json:"video_md5,omitempty"`
	ThumbMedia *CDNMedia `json:"thumb_media,omitempty"`
}

type ToolCallStartItem struct {
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type ToolCallResultItem struct {
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Status     string `json:"status,omitempty"`
}

type MessageItem struct {
	Type               int                 `json:"type,omitempty"`
	TextItem           *TextItem           `json:"text_item,omitempty"`
	ImageItem          *ImageItem          `json:"image_item,omitempty"`
	VoiceItem          *VoiceItem          `json:"voice_item,omitempty"`
	FileItem           *FileItem           `json:"file_item,omitempty"`
	VideoItem          *VideoItem          `json:"video_item,omitempty"`
	ToolCallStartItem  *ToolCallStartItem  `json:"tool_call_start_item,omitempty"`
	ToolCallResultItem *ToolCallResultItem `json:"tool_call_result_item,omitempty"`
	CreateTimeMs       int64               `json:"create_time_ms,omitempty"`
	IsCompleted        bool                `json:"is_completed,omitempty"`
}

type WeixinMessage struct {
	Seq           int64          `json:"seq,omitempty"`
	MessageID     int64          `json:"message_id,omitempty"`
	FromUserID    string         `json:"from_user_id,omitempty"`
	ToUserID      string         `json:"to_user_id,omitempty"`
	ClientID      string         `json:"client_id,omitempty"`
	CreateTimeMs  int64          `json:"create_time_ms,omitempty"`
	GroupID       string         `json:"group_id,omitempty"`
	MessageType   int            `json:"message_type,omitempty"`
	MessageState  int            `json:"message_state,omitempty"`
	ItemList      []*MessageItem `json:"item_list,omitempty"`
	ContextToken  string         `json:"context_token,omitempty"`
	RunID         string         `json:"run_id,omitempty"`
}

// API request/response types.

type GetUpdatesReq struct {
	GetUpdatesBuf string    `json:"get_updates_buf"`
	BaseInfo      *BaseInfo `json:"base_info"`
}

type GetUpdatesResp struct {
	Ret                  int              `json:"ret,omitempty"`
	Errcode              int              `json:"errcode,omitempty"`
	Errmsg               string           `json:"errmsg,omitempty"`
	Msgs                 []*WeixinMessage `json:"msgs,omitempty"`
	GetUpdatesBuf        string           `json:"get_updates_buf,omitempty"`
	LongpollingTimeoutMs int              `json:"longpolling_timeout_ms,omitempty"`
}

type SendMessageReq struct {
	Msg      *WeixinMessage `json:"msg"`
	BaseInfo *BaseInfo      `json:"base_info"`
}

type GetUploadUrlReq struct {
	FileKey     string    `json:"filekey,omitempty"`
	MediaType   int       `json:"media_type,omitempty"`
	ToUserID    string    `json:"to_user_id,omitempty"`
	RawSize     int       `json:"rawsize,omitempty"`
	RawFileMD5  string    `json:"rawfilemd5,omitempty"`
	FileSize    int       `json:"filesize,omitempty"`
	NoNeedThumb bool      `json:"no_need_thumb,omitempty"`
	AESKey      string    `json:"aeskey,omitempty"`
	BaseInfo    *BaseInfo `json:"base_info"`
}

type GetUploadUrlResp struct {
	UploadParam   string `json:"upload_param,omitempty"`
	UploadFullURL string `json:"upload_full_url,omitempty"`
}

type SendTypingReq struct {
	ILinkUserID  string    `json:"ilink_user_id,omitempty"`
	TypingTicket string    `json:"typing_ticket,omitempty"`
	Status       int       `json:"status,omitempty"` // 1=typing, 2=cancel
	BaseInfo     *BaseInfo `json:"base_info"`
}

type GetConfigReq struct {
	ILinkUserID  string    `json:"ilink_user_id,omitempty"`
	ContextToken string    `json:"context_token,omitempty"`
	BaseInfo     *BaseInfo `json:"base_info"`
}

type GetConfigResp struct {
	Ret          int    `json:"ret,omitempty"`
	Errmsg       string `json:"errmsg,omitempty"`
	TypingTicket string `json:"typing_ticket,omitempty"`
}

type NotifyReq struct {
	BaseInfo *BaseInfo `json:"base_info"`
}

type NotifyResp struct {
	Ret    int    `json:"ret,omitempty"`
	Errmsg string `json:"errmsg,omitempty"`
}

// QR code login types.

type QRCodeResp struct {
	QRCode            string `json:"qrcode"`
	QRCodeImgContent  string `json:"qrcode_img_content"`
}

type QRStatusResp struct {
	Status       string `json:"status"` // wait, scaned, confirmed, expired
	BotToken     string `json:"bot_token,omitempty"`
	ILinkBotID   string `json:"ilink_bot_id,omitempty"`
	BaseURL      string `json:"baseurl,omitempty"`
	ILinkUserID  string `json:"ilink_user_id,omitempty"`
	RedirectHost string `json:"redirect_host,omitempty"`
}
