package authchallenge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mythologyli/zju-connect/internal/securefile"
	"github.com/mythologyli/zju-connect/internal/terminal"
)

type CLIOptions struct {
	Context                     context.Context
	Customer                    string
	Generation                  uint64
	Input                       io.Reader // Explicit reader for embedded frontends and tests.
	Output                      io.Writer
	CaptchaTimeout              time.Duration
	Never                       bool
	CaptchaMode, CaptchaFile    string
	CallbackListen, CallbackURL string
	OnWaiting                   func(string)
	OnResumed                   func()
}

type CLIHandler struct {
	*SessionHandler
	options CLIOptions
	input   *bufio.Reader
}

func NewCLIHandler(options CLIOptions) *CLIHandler {
	if options.Output == nil {
		options.Output = os.Stderr
	}
	if options.CaptchaMode == "" {
		options.CaptchaMode = "auto"
	}
	if options.CallbackListen == "" {
		options.CallbackListen = "127.0.0.1:0"
	}
	h := &CLIHandler{options: options}
	if options.Input != nil {
		h.input = bufio.NewReaderSize(options.Input, terminal.MaxInput+1)
	}
	h.SessionHandler = NewSessionHandler(SessionOptions{Context: options.Context, Customer: options.Customer, Generation: options.Generation, Timeout: options.CaptchaTimeout, Responder: h, OnWaiting: options.OnWaiting, OnResumed: options.OnResumed})
	return h
}

func (h *CLIHandler) Respond(ctx context.Context, c Challenge) (Response, error) {
	r := Response{Meta: c.Meta}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if h.options.Never || (h.input == nil && !terminal.Available()) {
		return r, terminal.ErrInteractionRequired
	}
	fmt.Fprintf(h.options.Output, "[%s / session %d / %s] %s\n", terminal.SafeText(c.Meta.Customer), c.Meta.Generation, c.Meta.ID, terminal.SafeText(c.Kind))
	path := h.options.CaptchaFile
	if path == "" {
		path = c.OutputPath
	}
	if len(c.Image) > 0 {
		if _, err := imageSize(c.Image); err != nil {
			return r, err
		}
		if path != "" {
			if err := securefile.Write(path, c.Image); err != nil {
				return r, fmt.Errorf("write private captcha image: %w", err)
			}
			fmt.Fprintf(h.options.Output, "Original captcha: %s\n", terminal.SafeText(path))
		}
	}
	switch c.Kind {
	case "text-captcha":
		if h.options.CaptchaMode != "file" {
			if err := displayImage(h.options.Output, c.Image, h.options.CaptchaMode); err != nil {
				return r, err
			}
		}
	case "click-captcha":
		if h.options.CaptchaMode != "file" && c.OutputPath == "" {
			points, err := h.clickInBrowser(ctx, c)
			r.Click = points
			return r, err
		}
		if path == "" {
			return r, errors.New("captcha_file is required for offline click input")
		}
		value, err := h.readLine(ctx, c.Message, false)
		if err != nil {
			return r, err
		}
		points, err := parseClickCaptchaResponse(value)
		if err == nil {
			err = validateImagePoints(c.Image, points)
		}
		r.Click = points
		return r, err
	case "cas", "oauth2":
		link, err := url.Parse(c.URL)
		if err != nil || link.Scheme != "https" || link.Host == "" || link.User != nil {
			return r, errors.New("external login requires a valid HTTPS URL")
		}
		fmt.Fprintf(h.options.Output, "Open authentication link: %s\n", terminal.SafeText(c.URL))
	}
	if c.CanSkipSecondaryAuth {
		fmt.Fprintln(h.options.Output, "Add prefix '$' to request skipping secondary authentication")
	}
	value, err := h.readLine(ctx, c.Message, true)
	if err != nil {
		return r, err
	}
	r.Value = strings.TrimSpace(value)
	if c.CanSkipSecondaryAuth {
		if value, ok := strings.CutPrefix(r.Value, "$"); ok {
			r.Value = strings.TrimSpace(value)
			r.SkipSecondaryAuth = true
		}
	}
	if r.Value == "" {
		return r, errors.New("authentication response is empty")
	}
	return r, nil
}

func (h *CLIHandler) readLine(ctx context.Context, prompt string, hidden bool) (string, error) {
	if h.input == nil {
		return terminal.ReadLine(ctx, h.options.Output, prompt, hidden)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	fmt.Fprint(h.options.Output, terminal.SafeText(prompt)+" ")
	line, err := h.input.ReadSlice('\n')
	if len(line) > terminal.MaxInput || errors.Is(err, bufio.ErrBufferFull) {
		return "", errors.New("authentication input exceeds size limit")
	}
	if err != nil && !(err == io.EOF && len(line) > 0) {
		return "", err
	}
	return strings.TrimRight(string(line), "\r\n"), nil
}
func parseClickCaptchaResponse(raw string) (ClickCaptchaResponse, error) {
	if len(raw) > terminal.MaxInput {
		return ClickCaptchaResponse{}, errors.New("captcha response exceeds size limit")
	}
	var object struct {
		Coordinates [][]int `json:"coordinates"`
		Width       int     `json:"width"`
		Height      int     `json:"height"`
	}
	if json.Unmarshal([]byte(raw), &object) == nil && len(object.Coordinates) > 0 {
		response, err := pointsFromCoordinates(object.Coordinates)
		if err != nil {
			return ClickCaptchaResponse{}, err
		}
		response.Width = object.Width
		response.Height = object.Height
		return response, nil
	}

	var points []Point
	if json.Unmarshal([]byte(raw), &points) == nil && len(points) > 0 {
		return validatePoints(points)
	}

	var coordinates [][]int
	if json.Unmarshal([]byte(raw), &coordinates) == nil && len(coordinates) > 0 {
		return pointsFromCoordinates(coordinates)
	}
	return ClickCaptchaResponse{}, fmt.Errorf("unsupported click captcha response format")
}

func pointsFromCoordinates(coordinates [][]int) (ClickCaptchaResponse, error) {
	points := make([]Point, 0, len(coordinates))
	for _, coordinate := range coordinates {
		if len(coordinate) != 2 {
			return ClickCaptchaResponse{}, fmt.Errorf("invalid click captcha coordinate")
		}
		points = append(points, Point{X: coordinate[0], Y: coordinate[1]})
	}
	return validatePoints(points)
}

func validatePoints(points []Point) (ClickCaptchaResponse, error) {
	if len(points) == 0 || len(points) > 16 {
		return ClickCaptchaResponse{}, fmt.Errorf("click captcha response is empty")
	}
	for _, p := range points {
		if p.X < 0 || p.Y < 0 || p.X >= 4096 || p.Y >= 4096 {
			return ClickCaptchaResponse{}, errors.New("invalid click captcha coordinate")
		}
	}
	return ClickCaptchaResponse{Points: points}, nil
}
