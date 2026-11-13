package mcp

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"sync"
)

// ValidateArgs проверяет аргументы по JSON Schema из спецификации инструмента.
// Поддерживается подмножество, достаточное для контрактов /specs: type, required,
// properties, additionalProperties, enum, pattern, minLength, maxLength, minimum, maximum, items.
func ValidateArgs(schema map[string]any, args map[string]any) []string {
	if args == nil {
		args = map[string]any{}
	}
	var errs []string
	validateValue("", schema, args, &errs)
	sort.Strings(errs)
	return errs
}

var (
	patternMu    sync.Mutex
	patternCache = map[string]*regexp.Regexp{}
)

func compile(pattern string) (*regexp.Regexp, error) {
	patternMu.Lock()
	defer patternMu.Unlock()
	if re, ok := patternCache[pattern]; ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	patternCache[pattern] = re
	return re, nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func fieldName(path string) string {
	if path == "" {
		return "аргументы"
	}
	return path
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func validateValue(path string, schema map[string]any, value any, errs *[]string) {
	if schema == nil {
		return
	}
	typ, _ := schema["type"].(string)
	switch typ {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			*errs = append(*errs, fmt.Sprintf("%s: ожидается объект", fieldName(path)))
			return
		}
		props, _ := schema["properties"].(map[string]any)
		if required, ok := schema["required"].([]any); ok {
			for _, r := range required {
				name, _ := r.(string)
				if v, present := obj[name]; !present || v == nil {
					*errs = append(*errs, fmt.Sprintf("%s: обязательное поле", join(path, name)))
				}
			}
		}
		for key, v := range obj {
			sub, known := props[key].(map[string]any)
			if !known {
				if extra, ok := schema["additionalProperties"].(bool); ok && !extra {
					*errs = append(*errs, fmt.Sprintf("%s: неизвестное поле", join(path, key)))
				}
				continue
			}
			if v == nil {
				continue
			}
			validateValue(join(path, key), sub, v, errs)
		}
	case "string":
		s, ok := value.(string)
		if !ok {
			*errs = append(*errs, fmt.Sprintf("%s: ожидается строка", fieldName(path)))
			return
		}
		n := float64(len([]rune(s)))
		if min, ok := toFloat(schema["minLength"]); ok && n < min {
			*errs = append(*errs, fmt.Sprintf("%s: минимум %d симв.", fieldName(path), int(min)))
		}
		if max, ok := toFloat(schema["maxLength"]); ok && n > max {
			*errs = append(*errs, fmt.Sprintf("%s: максимум %d симв.", fieldName(path), int(max)))
		}
		if pattern, ok := schema["pattern"].(string); ok {
			re, err := compile(pattern)
			if err != nil {
				*errs = append(*errs, fmt.Sprintf("%s: некорректный pattern в спецификации", fieldName(path)))
			} else if !re.MatchString(s) {
				*errs = append(*errs, fmt.Sprintf("%s: значение %q не соответствует формату", fieldName(path), s))
			}
		}
		checkEnum(path, schema, s, errs)
	case "integer", "number":
		f, ok := toFloat(value)
		if !ok {
			*errs = append(*errs, fmt.Sprintf("%s: ожидается число", fieldName(path)))
			return
		}
		if typ == "integer" && f != math.Trunc(f) {
			*errs = append(*errs, fmt.Sprintf("%s: ожидается целое число", fieldName(path)))
		}
		if min, ok := toFloat(schema["minimum"]); ok && f < min {
			*errs = append(*errs, fmt.Sprintf("%s: минимум %v", fieldName(path), min))
		}
		if max, ok := toFloat(schema["maximum"]); ok && f > max {
			*errs = append(*errs, fmt.Sprintf("%s: максимум %v", fieldName(path), max))
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			*errs = append(*errs, fmt.Sprintf("%s: ожидается true/false", fieldName(path)))
		}
	case "array":
		var list []any
		switch v := value.(type) {
		case []any:
			list = v
		case []string:
			for _, s := range v {
				list = append(list, s)
			}
		default:
			*errs = append(*errs, fmt.Sprintf("%s: ожидается массив", fieldName(path)))
			return
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, it := range list {
				validateValue(fmt.Sprintf("%s[%d]", path, i), items, it, errs)
			}
		}
	}
}

func checkEnum(path string, schema map[string]any, s string, errs *[]string) {
	enum, ok := schema["enum"].([]any)
	if !ok {
		return
	}
	for _, e := range enum {
		if es, _ := e.(string); es == s {
			return
		}
	}
	*errs = append(*errs, fmt.Sprintf("%s: значение %q не входит в допустимый список", fieldName(path), s))
}

// argString достаёт строковый аргумент.
func argString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

// argStrings достаёт массив строк.
func argStrings(args map[string]any, key string) []string {
	switch v := args[key].(type) {
	case []string:
		return append([]string{}, v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, it := range v {
			if s, ok := it.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// argInt достаёт целочисленный аргумент или значение по умолчанию.
func argInt(args map[string]any, key string, def int) int {
	if f, ok := toFloat(args[key]); ok {
		return int(f)
	}
	return def
}

// toMap превращает структуру в map через JSON (результат инструмента).
func toMap(v any) map[string]any {
	raw, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}
