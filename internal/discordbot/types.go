package discordbot

// Discord interaction types (the inbound request's "type" field).
const (
	InteractionTypePing               = 1
	InteractionTypeApplicationCommand = 2
)

// Discord interaction response types (this bot's reply's "type" field).
const (
	ResponseTypePong                             = 1
	ResponseTypeChannelMessageWithSource         = 4
	ResponseTypeDeferredChannelMessageWithSource = 5
)

// FlagEphemeral marks a channel-message response visible only to the invoking
// user, so a link token or an error never lands in the public channel.
const FlagEphemeral = 64

// CommandOptionTypeString is the Discord application-command option type for a
// free-text string argument — the only option type this bot's commands use.
const CommandOptionTypeString = 3

// Interaction is the inbound POST body Discord sends for a PING or an
// application-command invocation. Only the fields this bot reads are typed;
// everything else Discord sends is ignored by json.Unmarshal.
type Interaction struct {
	Type   int              `json:"type"`
	Token  string           `json:"token"`
	Data   *InteractionData `json:"data,omitempty"`
	Member *Member          `json:"member,omitempty"`
	User   *User            `json:"user,omitempty"`
}

// InteractionData carries the invoked command's name and its arguments.
type InteractionData struct {
	Name    string              `json:"name"`
	Options []InteractionOption `json:"options,omitempty"`
}

// InteractionOption is one command argument. This bot's commands take a
// single string option each, so Value is typed as a string rather than the
// json.RawMessage a fully general Discord client would need.
type InteractionOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Member wraps User for interactions that fire inside a guild (as opposed to
// a DM, which sets User directly on the Interaction).
type Member struct {
	User *User `json:"user"`
}

// User carries the Discord snowflake id of the invoking account.
type User struct {
	ID string `json:"id"`
}

// Response is the JSON body this bot returns synchronously to Discord's HTTP
// POST — either a PONG, a deferred ack, or an immediate ephemeral message.
type Response struct {
	Type int           `json:"type"`
	Data *ResponseData `json:"data,omitempty"`
}

// ResponseData is the content of a channel-message response.
type ResponseData struct {
	Content string `json:"content,omitempty"`
	Flags   int    `json:"flags,omitempty"`
}

// PongResponse answers a PING interaction, as Discord's verification
// handshake requires.
func PongResponse() Response {
	return Response{Type: ResponseTypePong}
}

// DeferredResponse acknowledges an application command within Discord's
// 3-second window, buying time for the handler to do real work before
// calling EditOriginalResponse.
func DeferredResponse() Response {
	return Response{Type: ResponseTypeDeferredChannelMessageWithSource}
}

// EphemeralResponse replies immediately with content visible only to the
// invoking user.
func EphemeralResponse(content string) Response {
	return Response{
		Type: ResponseTypeChannelMessageWithSource,
		Data: &ResponseData{Content: content, Flags: FlagEphemeral},
	}
}

// Command describes one slash command for registration via RegisterCommands.
type Command struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Options     []CommandOption `json:"options,omitempty"`
}

// CommandOption describes one argument of a Command. This bot only needs
// required string options (/link token:<string>, /contribute url:<string>).
type CommandOption struct {
	Type        int    `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}
