package llm

// Role identifica quién habla en un mensaje.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message es un turno de conversación.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}
