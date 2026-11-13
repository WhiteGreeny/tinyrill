package bounded

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

func XML(data string) error {
	if len(data) > MaxResponse {
		return ErrTooLarge
	}
	decoder := xml.NewDecoder(strings.NewReader(data))
	depth, count := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("invalid gateway XML")
		}
		count++
		if count > 100000 {
			return errors.New("gateway XML node limit exceeded")
		}
		switch token.(type) {
		case xml.StartElement:
			depth++
			if depth > 64 {
				return errors.New("gateway XML nesting limit exceeded")
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			return errors.New("gateway XML directives are not supported")
		}
	}
}
