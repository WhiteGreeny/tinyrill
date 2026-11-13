package credentials

import (
	"context"
	"errors"
	"fmt"
	"github.com/godbus/dbus/v5"
	"os"
	"strings"
	"time"
)

type systemBackend struct{}

const secretBus = "org.freedesktop.secrets"
const secretService = "org.freedesktop.Secret.Service"
const secretItem = "org.freedesktop.Secret.Item"

type secretValue struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}
type secretConnection struct {
	conn    *dbus.Conn
	session dbus.ObjectPath
}

func openSecretService(ctx context.Context) (*secretConnection, error) {
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		address = fmt.Sprintf("unix:path=/run/user/%d/bus", os.Getuid())
	}
	// Secret Service's plain session is only permitted on a local Unix bus.
	for _, part := range strings.Split(address, ";") {
		if !strings.HasPrefix(part, "unix:") {
			return nil, ErrUnavailable
		}
	}
	c, err := dbus.Connect(address, dbus.WithContext(ctx))
	if err != nil {
		return nil, ErrUnavailable
	}
	var output dbus.Variant
	var session dbus.ObjectPath
	// "plain" is a standard Secret Service algorithm on the authenticated
	// local session bus; no password is exposed in a command or environment.
	err = c.Object(secretBus, "/org/freedesktop/secrets").CallWithContext(ctx, secretService+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&output, &session)
	if err != nil {
		c.Close()
		return nil, nativeError("open", ErrUnavailable)
	}
	return &secretConnection{c, session}, nil
}
func (s *secretConnection) close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.conn.Object(secretBus, s.session).CallWithContext(ctx, "org.freedesktop.Secret.Session.Close", 0)
	s.conn.Close()
}
func (s *secretConnection) find(ctx context.Context, key string) (dbus.ObjectPath, error) {
	var unlocked, locked []dbus.ObjectPath
	err := s.conn.Object(secretBus, "/org/freedesktop/secrets").CallWithContext(ctx, secretService+".SearchItems", 0, map[string]string{"application": "tinyrill", "key": key}).Store(&unlocked, &locked)
	if err != nil {
		return "", nativeError("search", ErrUnavailable)
	}
	if len(locked) > 0 {
		return "", ErrLocked
	}
	if len(unlocked) == 0 {
		return "", ErrNotFound
	}
	if len(unlocked) != 1 {
		return "", errors.New("multiple matching system credentials")
	}
	return unlocked[0], nil
}
func secretContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 15*time.Second)
}
func (systemBackend) Get(ctx context.Context, key string) ([]byte, error) {
	ctx, cancel := secretContext(ctx)
	defer cancel()
	s, err := openSecretService(ctx)
	if err != nil {
		return nil, err
	}
	defer s.close()
	path, err := s.find(ctx, key)
	if err != nil {
		return nil, err
	}
	var value secretValue
	if err := s.conn.Object(secretBus, path).CallWithContext(ctx, secretItem+".GetSecret", 0, s.session).Store(&value); err != nil {
		return nil, nativeError("read", ErrLocked)
	}
	if len(value.Value) > maxRecord || value.Session != s.session || len(value.Parameters) != 0 {
		clear(value.Value)
		return nil, errors.New("invalid Secret Service response")
	}
	return value.Value, nil
}
func (systemBackend) Set(ctx context.Context, key string, data []byte) error {
	ctx, cancel := secretContext(ctx)
	defer cancel()
	s, err := openSecretService(ctx)
	if err != nil {
		return err
	}
	defer s.close()
	path, err := s.find(ctx, key)
	value := secretValue{s.session, []byte{}, data, "application/json"}
	if err == nil {
		if err = s.conn.Object(secretBus, path).CallWithContext(ctx, secretItem+".SetSecret", 0, value).Err; err != nil {
			return nativeError("write", ErrLocked)
		}
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	var collection dbus.ObjectPath
	if err = s.conn.Object(secretBus, "/org/freedesktop/secrets").CallWithContext(ctx, secretService+".ReadAlias", 0, "default").Store(&collection); err != nil || collection == "/" {
		return ErrUnavailable
	}
	properties := map[string]dbus.Variant{secretItem + ".Label": dbus.MakeVariant("tinyrill saved VPN password"), secretItem + ".Attributes": dbus.MakeVariant(map[string]string{"application": "tinyrill", "key": key})}
	var item, prompt dbus.ObjectPath
	if err = s.conn.Object(secretBus, collection).CallWithContext(ctx, "org.freedesktop.Secret.Collection.CreateItem", 0, properties, value, true).Store(&item, &prompt); err != nil {
		return nativeError("create", ErrLocked)
	}
	if prompt != "/" || item == "/" {
		return ErrLocked
	}
	return nil
}
func (systemBackend) Delete(ctx context.Context, key string) error {
	ctx, cancel := secretContext(ctx)
	defer cancel()
	s, err := openSecretService(ctx)
	if err != nil {
		return err
	}
	defer s.close()
	path, err := s.find(ctx, key)
	if err != nil {
		return err
	}
	var prompt dbus.ObjectPath
	if err = s.conn.Object(secretBus, path).CallWithContext(ctx, secretItem+".Delete", 0).Store(&prompt); err != nil || prompt != "/" {
		return nativeError("delete", ErrLocked)
	}
	return nil
}
