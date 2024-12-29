package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// JSONObject represents a JSON object with ordered keys
type JSONObject struct {
	keys   []string
	values map[string]any
}

// Len returns the length of the JSON object
func (obj *JSONObject) Len() int {
	return len(obj.keys)
}

// Get returns the value of the key in the JSON object
func (obj *JSONObject) Get(key string) (any, bool) {
	v, ok := obj.values[key]
	return v, ok
}

// UnmarshalJSON implements type json.Unmarshaler interface
func (obj *JSONObject) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	t, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expect JSON object open with '{'")
	}

	err = obj.parse(dec)
	if err != nil {
		return err
	}

	t, err = dec.Token()
	if err != io.EOF {
		return fmt.Errorf("expect end of JSON object but got more token: %T: %v or err: %v", t, t, err)
	}

	return nil
}

func (obj *JSONObject) parse(dec *json.Decoder) (err error) {
	var t json.Token
	for dec.More() {
		t, err = dec.Token()
		if err != nil {
			return err
		}

		key, ok := t.(string)
		if !ok {
			return fmt.Errorf("expecting JSON key should be always a string: %T: %v", t, t)
		}

		t, err = dec.Token()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		var value any
		value, err = handleDelim(t, dec)
		if err != nil {
			return err
		}

		obj.keys = append(obj.keys, key)
		if obj.values == nil {
			obj.values = make(map[string]any)
		}
		obj.values[key] = value
	}

	t, err = dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); !ok || delim != '}' {
		return fmt.Errorf("expect JSON object close with '}'")
	}

	return nil
}

func parseArray(dec *json.Decoder) (arr []any, err error) {
	var t json.Token
	arr = make([]any, 0)
	for dec.More() {
		t, err = dec.Token()
		if err != nil {
			return
		}

		var value any
		value, err = handleDelim(t, dec)
		if err != nil {
			return
		}
		arr = append(arr, value)
	}
	t, err = dec.Token()
	if err != nil {
		return
	}
	if delim, ok := t.(json.Delim); !ok || delim != ']' {
		err = fmt.Errorf("expect JSON array close with ']'")
		return
	}

	return
}

func handleDelim(t json.Token, dec *json.Decoder) (res any, err error) {
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			obj := JSONObject{
				values: make(map[string]any),
			}
			err = obj.parse(dec)
			if err != nil {
				return
			}
			return obj, nil
		case '[':
			var value []any
			value, err = parseArray(dec)
			if err != nil {
				return
			}
			return value, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter: %q", delim)
		}
	}
	return t, nil
}

type JSONAny struct {
	Str string
	Map map[string]any
	Any any
}

func (a *JSONAny) MarshalJSON() ([]byte, error) {
	if a.Str != "" {
		return json.Marshal(a.Str)
	}
	if a.Map != nil {
		return json.Marshal(a.Map)
	}
	return json.Marshal(a.Any)
}

func (a *JSONAny) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		a.Str = s
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) == nil {
		a.Map = m
		return nil
	}
	return json.Unmarshal(b, &a.Any)
}

func (a *JSONAny) MainString() string {
	if a.Str != "" {
		return a.Str
	}
	if a.Map != nil {
		if v, ok := a.Map["."]; ok {
			if s, isStr := v.(string); isStr {
				return s
			}
		}
	}
	return ""
}
