package command

import (
	"image"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Command struct {
	Response string   `json:"response"`
	Aliases  []string `json:"aliases,omitempty"`
}

type Context struct {
	CommandName string
	Args        string
	UserID      string
	Username    string
	Response    string
	Respond     func(text string, img image.Image)
	DBPool      *pgxpool.Pool
}

type Handler func(ctx Context)

type ResponseFunc func(username, text string, img image.Image)

type Registry struct {
	commands   map[string]Command
	aliases    map[string]string
	handlers   map[string]Handler
	onResponse ResponseFunc
	dbPool     *pgxpool.Pool
}

func NewRegistry(commands map[string]Command, dbPool *pgxpool.Pool, onResponse ResponseFunc) *Registry {
	r := &Registry{
		commands:   commands,
		aliases:    make(map[string]string),
		handlers:   make(map[string]Handler),
		onResponse: onResponse,
		dbPool:     dbPool,
	}
	for name, cmd := range commands {
		for _, alias := range cmd.Aliases {
			r.aliases[strings.ToLower(alias)] = name
		}
	}
	return r
}

func (r *Registry) RegisterHandler(name string, handler Handler) {
	r.handlers[name] = handler
}

func (r *Registry) Dispatch(message, userID, username string) bool {
	message = strings.TrimSpace(message)
	if !strings.HasPrefix(message, "!") {
		return false
	}

	withoutPrefix := message[1:]
	name, args, _ := strings.Cut(withoutPrefix, " ")
	name = strings.ToLower(name)

	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}

	cmd, ok := r.commands[name]
	if !ok {
		return false
	}

	respond := func(text string, img image.Image) {
		if r.onResponse != nil {
			r.onResponse(username, text, img)
		}
	}

	ctx := Context{
		CommandName: name,
		Args:        strings.TrimSpace(args),
		UserID:      userID,
		Username:    username,
		Response:    cmd.Response,
		Respond:     respond,
		DBPool:      r.dbPool,
	}

	if handler, hok := r.handlers[name]; hok {
		handler(ctx)
		return true
	}

	if cmd.Response != "" {
		log.Printf("Command !%s triggered by %s", name, username)
		respond(cmd.Response, nil)
	}

	return true
}

func (r *Registry) GetCommand(name string) (Command, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}
