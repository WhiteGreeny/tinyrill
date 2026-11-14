package instance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mythologyli/zju-connect/client/authchallenge"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/internal/credentials"
	"github.com/mythologyli/zju-connect/internal/securefile"
	"github.com/mythologyli/zju-connect/internal/terminal"
	"github.com/mythologyli/zju-connect/log"
)

// CLIFrontend owns only foreground interaction and the optional password store.
// Secret-store reads are delayed until the server actually requires a password.
type CLIFrontend struct {
	Profile               configs.Profile
	Output, Diagnostic    io.Writer
	ResourceMode          string
	DebugResources        bool
	DebugResourceValues   bool
	store                 *credentials.Store
	password              string
	loaded, authenticated bool
	waiting               func(string)
	resumed               func()
	Prompt                credentials.Prompt
}

func (f *CLIFrontend) Status(s Status) { _ = json.NewEncoder(f.Output).Encode(s) }

func (f *CLIFrontend) prompt(ctx context.Context, label string) (string, error) {
	timeout, err := time.ParseDuration(f.Profile.Auth.Timeout)
	if err != nil || timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if f.Profile.Auth.Interactive == "never" {
		return "", terminal.ErrInteractionRequired
	}
	if f.Prompt != nil {
		return f.Prompt(ctx, label)
	}
	return terminal.ReadLine(ctx, f.Diagnostic, label, true)
}

func (f *CLIFrontend) Password(ctx context.Context) (string, error) {
	if f.loaded {
		return f.password, nil
	}
	var password string
	var err error
	if f.Profile.Auth.SecretFile != "" {
		password, err = ReadSecret(f.Profile.Auth.SecretFile)
	} else if f.Profile.Auth.RememberPassword {
		f.store, err = credentials.New(f.Profile, f.prompt)
		if err == nil {
			password, err = f.store.Get(ctx)
		}
		if errors.Is(err, credentials.ErrNotFound) {
			err = nil
		}
	}
	if err != nil {
		return "", err
	}
	if password == "" {
		if f.waiting != nil {
			f.waiting("password")
		}
		password, err = f.prompt(ctx, "VPN password:")
		if err != nil {
			return "", err
		}
		if f.resumed != nil {
			f.resumed()
		}
	}
	if password == "" || len(password) > 4096 {
		return "", errors.New("password must contain 1–4096 bytes")
	}
	f.password, f.loaded = password, true
	return password, nil
}

func (f *CLIFrontend) Challenge(ctx context.Context, generation uint64, waiting func(string), resumed func()) authchallenge.Handler {
	f.waiting, f.resumed = waiting, resumed
	path := f.Profile.Auth.CaptchaFile
	if path == "" {
		path = filepath.Join(f.Profile.Session.StateDir, "captcha.png")
	}
	return authchallenge.NewCLIHandler(authchallenge.CLIOptions{Context: ctx, Customer: f.Profile.ID, Generation: generation, Output: f.Diagnostic, CaptchaTimeout: duration(f.Profile.Auth.Timeout), Never: f.Profile.Auth.Interactive == "never", CaptchaMode: f.Profile.Auth.CaptchaMode, CaptchaFile: path, CallbackListen: f.Profile.Auth.CallbackListen, CallbackURL: f.Profile.Auth.CallbackURL, OnWaiting: waiting, OnResumed: resumed})
}

func (f *CLIFrontend) Authenticated(ctx context.Context) {
	if f.authenticated {
		return
	}
	f.authenticated = true
	if f.Profile.Auth.RememberPassword && f.loaded {
		if f.store == nil {
			f.store, _ = credentials.New(f.Profile, f.prompt)
		}
		if f.store == nil || f.store.Set(ctx, f.password) != nil {
			log.Println("Login succeeded, but the password was not saved")
		}
	}
	f.password = ""
}
func (f *CLIFrontend) Close() {
	f.password = ""
	if f.store != nil {
		f.store.Close()
	}
}

func ReadSecret(path string) (string, error) {
	data, err := securefile.ReadLimited(path, 4098)
	if err != nil {
		return "", err
	}
	defer clear(data)
	value := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if len(value) == 0 || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("secret file must contain one non-empty line of at most 4096 bytes")
	}
	return value, nil
}
