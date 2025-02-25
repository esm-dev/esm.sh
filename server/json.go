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

// StripJSONC strips out comments and trailing commas and convert the input to a
// valid JSON per the official spec: https://tools.ietf.org/html/rfc8259
//
// The resulting JSON will always be the same length as the input and it will
// include all of the same line breaks at matching offsets. This is to ensure
// the result can be later processed by a external parser and that that
// parser will report messages or errors with the correct offsets.
//
// The MIT License (MIT)
// Copyright (c) 2021 Josh Baker
func StripJSONC(src []byte) []byte {
	var dst []byte
	for i := 0; i < len(src); i++ {
		if src[i] == '/' {
			if i < len(src)-1 {
				if src[i+1] == '/' {
					dst = append(dst, ' ', ' ')
					i += 2
					for ; i < len(src); i++ {
						if src[i] == '\n' {
							dst = append(dst, '\n')
							break
						} else if src[i] == '\t' || src[i] == '\r' {
							dst = append(dst, src[i])
						} else {
							dst = append(dst, ' ')
						}
					}
					continue
				}
				if src[i+1] == '*' {
					dst = append(dst, ' ', ' ')
					i += 2
					for ; i < len(src)-1; i++ {
						if src[i] == '*' && src[i+1] == '/' {
							dst = append(dst, ' ', ' ')
							i++
							break
						} else if src[i] == '\n' || src[i] == '\t' ||
							src[i] == '\r' {
							dst = append(dst, src[i])
						} else {
							dst = append(dst, ' ')
						}
					}
					continue
				}
			}
		}
		dst = append(dst, src[i])
		if src[i] == '"' {
			for i = i + 1; i < len(src); i++ {
				dst = append(dst, src[i])
				if src[i] == '"' {
					j := i - 1
					for ; ; j-- {
						if src[j] != '\\' {
							break
						}
					}
					if (j-i)%2 != 0 {
						break
					}
				}
			}
		} else if src[i] == '}' || src[i] == ']' {
			for j := len(dst) - 2; j >= 0; j-- {
				if dst[j] <= ' ' {
					continue
				}
				if dst[j] == ',' {
					dst[j] = ' '
				}
				break
			}
		}
	}
	return dst
}
