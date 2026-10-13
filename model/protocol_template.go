package model

// ProtocolTemplate describes a bounded HTTP media workflow, without credentials.
type ProtocolTemplate struct {
	ID             string                       `json:"id"`
	Name           string                       `json:"name"`
	Revision       int                          `json:"revision"`
	DefaultBaseURL string                       `json:"defaultBaseUrl"`
	Headers        map[string]string            `json:"headers"`
	Models         *ProtocolModels              `json:"models,omitempty"`
	Upload         *ProtocolUpload              `json:"upload,omitempty"`
	Operations     map[string]ProtocolOperation `json:"operations"`
}

type ProtocolRequest struct {
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Encoding  string            `json:"encoding,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      map[string]any    `json:"body,omitempty"`
	FileField string            `json:"fileField,omitempty"`
	Response  ProtocolResponse  `json:"response"`
}

type ProtocolResponse struct {
	Format       string   `json:"format"`
	TaskID       string   `json:"taskId,omitempty"`
	Status       string   `json:"status,omitempty"`
	Running      []string `json:"running,omitempty"`
	Success      []string `json:"success,omitempty"`
	Failure      []string `json:"failure,omitempty"`
	ResultURL    string   `json:"resultUrl,omitempty"`
	ResultBase64 string   `json:"resultBase64,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type ProtocolModels struct {
	Request ProtocolRequest `json:"request"`
	Items   string          `json:"items"`
	ID      string          `json:"id"`
}

type ProtocolUpload struct {
	Mode    string          `json:"mode"`
	Request ProtocolRequest `json:"request"`
	URL     string          `json:"url,omitempty"`
	Key     string          `json:"key,omitempty"`
}

type ProtocolOperation struct {
	Create         ProtocolRequest           `json:"create"`
	Resume         *ProtocolRequest          `json:"resume,omitempty"`
	TimeoutSeconds int                       `json:"timeoutSeconds"`
	PollSeconds    int                       `json:"pollSeconds"`
	MaxImages      int                       `json:"maxImages"`
	ValueMaps      map[string]map[string]any `json:"valueMaps,omitempty"`
}

// GenerationProtocolSnapshot freezes how an accepted task is resumed. Secrets
// remain on the channel; temporary media URLs never enter this snapshot.
type GenerationProtocolSnapshot struct {
	ChannelID  string           `json:"channelId"`
	BaseURL    string           `json:"baseUrl"`
	Template   ProtocolTemplate `json:"template"`
	Operation  string           `json:"operation"`
	DeadlineAt string           `json:"deadlineAt"`
}
