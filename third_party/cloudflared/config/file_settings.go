package config

import (
	"fmt"
	"time"
)

type configFileSettings struct {
	Configuration `yaml:",inline"`
	// older settings will be aggregated into the generic map, should be read via cli.Context
	Settings map[string]interface{} `yaml:",inline"`
}

func (c *configFileSettings) Int(name string) (int, error) {
	if raw, ok := c.Settings[name]; ok {
		if v, ok := raw.(int); ok {
			return v, nil
		}
		return 0, fmt.Errorf("expected int found %T for %s", raw, name)
	}
	return 0, nil
}

func (c *configFileSettings) Duration(name string) (time.Duration, error) {
	if raw, ok := c.Settings[name]; ok {
		switch v := raw.(type) {
		case time.Duration:
			return v, nil
		case string:
			return time.ParseDuration(v)
		}
		return 0, fmt.Errorf("expected duration found %T for %s", raw, name)
	}
	return 0, nil
}

func (c *configFileSettings) Float64(name string) (float64, error) {
	if raw, ok := c.Settings[name]; ok {
		if v, ok := raw.(float64); ok {
			return v, nil
		}
		return 0, fmt.Errorf("expected float found %T for %s", raw, name)
	}
	return 0, nil
}

func (c *configFileSettings) String(name string) (string, error) {
	if raw, ok := c.Settings[name]; ok {
		if v, ok := raw.(string); ok {
			return v, nil
		}
		return "", fmt.Errorf("expected string found %T for %s", raw, name)
	}
	return "", nil
}

func (c *configFileSettings) StringSlice(name string) ([]string, error) {
	if raw, ok := c.Settings[name]; ok {
		if slice, ok := raw.([]interface{}); ok {
			strSlice := make([]string, len(slice))
			for i, v := range slice {
				str, ok := v.(string)
				if !ok {
					return nil, fmt.Errorf("expected string, found %T for %v", i, v)
				}
				strSlice[i] = str
			}
			return strSlice, nil
		}
		return nil, fmt.Errorf("expected string slice found %T for %s", raw, name)
	}
	return nil, nil
}

func (c *configFileSettings) IntSlice(name string) ([]int, error) {
	if raw, ok := c.Settings[name]; ok {
		if slice, ok := raw.([]interface{}); ok {
			intSlice := make([]int, len(slice))
			for i, v := range slice {
				str, ok := v.(int)
				if !ok {
					return nil, fmt.Errorf("expected int, found %T for %v ", v, v)
				}
				intSlice[i] = str
			}
			return intSlice, nil
		}
		if v, ok := raw.([]int); ok {
			return v, nil
		}
		return nil, fmt.Errorf("expected int slice found %T for %s", raw, name)
	}
	return nil, nil
}

func (c *configFileSettings) Bool(name string) (bool, error) {
	if raw, ok := c.Settings[name]; ok {
		if v, ok := raw.(bool); ok {
			return v, nil
		}
		return false, fmt.Errorf("expected boolean found %T for %s", raw, name)
	}
	return false, nil
}
