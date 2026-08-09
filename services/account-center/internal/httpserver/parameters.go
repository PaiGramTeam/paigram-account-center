package httpserver

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

type Parameter struct {
	name     string
	location string
	required bool
	schema   *huma.Schema
}

func PathID(name string) Parameter {
	minimum := float64(1)
	return Parameter{
		name: name, location: "path", required: true,
		schema: &huma.Schema{Type: huma.TypeInteger, Format: "int64", Minimum: &minimum},
	}
}

func PathString(name string) Parameter {
	minimum := 1
	return Parameter{
		name: name, location: "path", required: true,
		schema: &huma.Schema{Type: huma.TypeString, MinLength: &minimum},
	}
}

func QueryInteger(name string, defaultValue, minimum, maximum int) Parameter {
	schema := &huma.Schema{Type: huma.TypeInteger, Format: "int32", Default: defaultValue}
	if minimum != 0 {
		value := float64(minimum)
		schema.Minimum = &value
	}
	if maximum != 0 {
		value := float64(maximum)
		schema.Maximum = &value
	}
	return Parameter{name: name, location: "query", schema: schema}
}

func QueryString(name string, allowed ...string) Parameter {
	schema := &huma.Schema{Type: huma.TypeString}
	for _, value := range allowed {
		schema.Enum = append(schema.Enum, value)
	}
	return Parameter{name: name, location: "query", schema: schema}
}

func QueryDate(name string) Parameter {
	return Parameter{name: name, location: "query", schema: &huma.Schema{Type: huma.TypeString, Format: "date"}}
}

func RequiredQueryString(name string) Parameter {
	parameter := QueryString(name)
	parameter.required = true
	minimum := 1
	parameter.schema.MinLength = &minimum
	return parameter
}

func QueryBoolean(name string, defaultValue bool) Parameter {
	return Parameter{name: name, location: "query", schema: &huma.Schema{
		Type: huma.TypeBoolean, Default: defaultValue,
	}}
}

func (c Contract) WithParameters(parameters ...Parameter) Contract {
	c.parameters = append(append([]Parameter(nil), c.parameters...), parameters...)
	return c
}

func (c Contract) prepareParameters(path string) Contract {
	for _, match := range humaPathParameterPattern.FindAllStringSubmatch(path, -1) {
		name := match[1]
		if c.hasParameter("path", name) {
			continue
		}
		if strings.EqualFold(name, "id") || strings.HasSuffix(name, "Id") {
			c.parameters = append(c.parameters, PathID(name))
		} else {
			c.parameters = append(c.parameters, PathString(name))
		}
	}
	return c
}

func (c Contract) hasParameter(location, name string) bool {
	for _, parameter := range c.parameters {
		if parameter.location == location && parameter.name == name {
			return true
		}
	}
	return false
}

func (c Contract) documentParameters(op *huma.Operation) {
	op.Parameters = make([]*huma.Param, 0, len(c.parameters))
	for _, parameter := range c.parameters {
		op.Parameters = append(op.Parameters, &huma.Param{
			Name: parameter.name, In: parameter.location, Required: parameter.required, Schema: parameter.schema,
		})
	}
}

func (c Contract) validateParameters(context *gin.Context) *contractValidationFailure {
	for _, parameter := range c.parameters {
		value := context.Query(parameter.name)
		if parameter.location == "path" {
			value = context.Param(parameter.name)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			if parameter.required {
				return parameterFailure(parameter, "required parameter is missing")
			}
			continue
		}
		if err := validateParameterValue(parameter, value); err != nil {
			return parameterFailure(parameter, err.Error())
		}
	}
	return nil
}

func validateParameterValue(parameter Parameter, raw string) error {
	switch parameter.schema.Type {
	case huma.TypeInteger:
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("expected integer: %w", err)
		}
		if parameter.schema.Minimum != nil && float64(value) < *parameter.schema.Minimum {
			return fmt.Errorf("must be at least %v", *parameter.schema.Minimum)
		}
		if parameter.schema.Maximum != nil && float64(value) > *parameter.schema.Maximum {
			return fmt.Errorf("must be at most %v", *parameter.schema.Maximum)
		}
	case huma.TypeBoolean:
		if _, err := strconv.ParseBool(raw); err != nil {
			return fmt.Errorf("expected boolean: %w", err)
		}
	case huma.TypeString:
		if parameter.schema.MinLength != nil && len(raw) < *parameter.schema.MinLength {
			return fmt.Errorf("must contain at least %d characters", *parameter.schema.MinLength)
		}
		if len(parameter.schema.Enum) > 0 && !enumContains(parameter.schema.Enum, raw) {
			return fmt.Errorf("must be one of the documented values")
		}
		if parameter.schema.Format == "date" {
			if _, err := time.Parse(time.DateOnly, raw); err != nil {
				return fmt.Errorf("expected ISO 8601 date: %w", err)
			}
		}
	}
	return nil
}

func enumContains(values []any, raw string) bool {
	for _, value := range values {
		if candidate, ok := value.(string); ok && candidate == raw {
			return true
		}
	}
	return false
}

func parameterFailure(parameter Parameter, message string) *contractValidationFailure {
	detail := fmt.Sprintf("%s.%s: %s", parameter.location, parameter.name, message)
	return &contractValidationFailure{
		status: http.StatusBadRequest, message: "request parameter validation failed", details: []string{detail},
	}
}
