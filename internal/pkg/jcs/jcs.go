package jcs

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
)

// Format returns the JSON Canonicalization Scheme (JCS) RFC 8785 representation of the given value.
func Format(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}

	switch val := v.(type) {
	case map[string]interface{}:
		return formatMap(val)
	case []interface{}:
		return formatSlice(val)
	case string:
		return formatString(val)
	case bool:
		return formatBool(val)
	case float64:
		return formatFloat(val)
	case int:
		return []byte(strconv.Itoa(val)), nil
	case int64:
		return []byte(strconv.FormatInt(val, 10)), nil
	case json.Number:
		return []byte(val.String()), nil
	default:
		return formatFallback(val)
	}
}

func formatMap(val map[string]interface{}) ([]byte, error) {
	var keys []string
	for k := range val {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := Format(k)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')

		valBytes, err := Format(val[k])
		if err != nil {
			return nil, err
		}
		buf.Write(valBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func formatSlice(val []interface{}) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, elem := range val {
		if i > 0 {
			buf.WriteByte(',')
		}
		elemBytes, err := Format(elem)
		if err != nil {
			return nil, err
		}
		buf.Write(elemBytes)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

func formatString(val string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(val); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func formatBool(val bool) ([]byte, error) {
	if val {
		return []byte("true"), nil
	}
	return []byte("false"), nil
}

func formatFloat(val float64) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(val); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func formatFallback(v interface{}) ([]byte, error) {
	bytesVal, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var genericVal interface{}
	d := json.NewDecoder(bytes.NewReader(bytesVal))
	d.UseNumber()
	if err := d.Decode(&genericVal); err != nil {
		return nil, err
	}
	return Format(genericVal)
}
