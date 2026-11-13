package authchallenge

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"time"
)

type Metadata struct {
	Customer   string    `json:"customer"`
	Generation uint64    `json:"generation"`
	ID         string    `json:"id"`
	Expires    time.Time `json:"expires"`
}

// Challenge and Response form the UI boundary. A response can only complete
// its originating customer, session generation and single-use challenge.
type Challenge struct {
	Meta                           Metadata
	Kind, Message, URL, OutputPath string
	Image                          []byte
	CanSkipSecondaryAuth           bool
}
type Response struct {
	Meta              Metadata
	Value             string
	Click             ClickCaptchaResponse
	SkipSecondaryAuth bool
}

type Responder interface {
	Respond(context.Context, Challenge) (Response, error)
}
type ResponderFunc func(context.Context, Challenge) (Response, error)

func (f ResponderFunc) Respond(ctx context.Context, c Challenge) (Response, error) { return f(ctx, c) }

type SessionOptions struct {
	Context    context.Context
	Customer   string
	Generation uint64
	Timeout    time.Duration
	Responder  Responder
	OnWaiting  func(string)
	OnResumed  func()
}
type SessionHandler struct {
	options SessionOptions
	mu      sync.Mutex
}

func NewSessionHandler(options SessionOptions) *SessionHandler {
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Minute
	}
	return &SessionHandler{options: options}
}

func (h *SessionHandler) exchange(c Challenge) (Response, error) {
	if !h.mu.TryLock() {
		return Response{}, errors.New("another authentication challenge is pending")
	}
	defer h.mu.Unlock()
	ctx, cancel := context.WithTimeout(h.options.Context, h.options.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	expires, _ := ctx.Deadline()
	c.Meta = Metadata{Customer: h.options.Customer, Generation: h.options.Generation, ID: rand.Text(), Expires: expires}
	if h.options.OnWaiting != nil {
		h.options.OnWaiting(c.Kind)
	}
	if h.options.Responder == nil {
		return Response{}, errors.New("authentication responder is unavailable")
	}
	response, err := h.options.Responder.Respond(ctx, c)
	if err != nil {
		return Response{}, err
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if response.Meta != c.Meta {
		return Response{}, errors.New("authentication response belongs to a different challenge")
	}
	if h.options.OnResumed != nil {
		h.options.OnResumed()
	}
	return response, nil
}

func (h *SessionHandler) HandleCodeChallenge(c CodeChallenge) (CodeResponse, error) {
	r, err := h.exchange(Challenge{Kind: string(c.Kind), Message: c.Message, CanSkipSecondaryAuth: c.CanSkipSecondaryAuth})
	return CodeResponse{Code: r.Value, SkipSecondaryAuth: r.SkipSecondaryAuth}, err
}
func (h *SessionHandler) HandleTextCaptcha(c TextCaptchaChallenge) (TextCaptchaResponse, error) {
	r, err := h.exchange(Challenge{Kind: "text-captcha", Message: c.Message, Image: c.Image, OutputPath: c.OutputPath})
	return TextCaptchaResponse{Code: r.Value}, err
}
func (h *SessionHandler) HandleClickCaptcha(c ClickCaptchaChallenge) (ClickCaptchaResponse, error) {
	r, err := h.exchange(Challenge{Kind: "click-captcha", Message: c.Message, Image: c.Image, OutputPath: c.OutputPath})
	return r.Click, err
}
func (h *SessionHandler) HandleExternalLogin(c ExternalLoginChallenge) (ExternalLoginResponse, error) {
	r, err := h.exchange(Challenge{Kind: string(c.Kind), Message: c.Message, URL: c.LoginURL})
	return ExternalLoginResponse{CallbackURL: r.Value}, err
}
