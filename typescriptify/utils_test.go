package typescriptify

import "testing"

func TestIndentLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		indent   int
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			indent:   0,
			expected: "",
		},
		{
			name:     "single",
			input:    "a",
			indent:   0,
			expected: "a",
		},
		{
			name:     "single indent",
			input:    "a",
			indent:   1,
			expected: "\ta",
		},
		{
			name:     "multi",
			input:    "a\nb\nc",
			indent:   0,
			expected: "a\nb\nc",
		},
		{
			name:     "multi indent",
			input:    "a\nb\nc",
			indent:   1,
			expected: "\ta\n\tb\n\tc",
		},
		{
			name:     "multi indent 2",
			input:    "a\nb\nc",
			indent:   2,
			expected: "\t\ta\n\t\tb\n\t\tc",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := indentLines(test.input, test.indent)
			if actual != test.expected {
				t.Errorf("expected %q, got %q", test.expected, actual)
			}
		})
	}
}

func TestCamelCase(t *testing.T) {
	t.Parallel()

	// ref https://github.com/sindresorhus/camelcase/blob/main/test.js
	tests := []struct {
		input    string
		expected string
	}{
		{
			// empty
			input:    "",
			expected: "",
		},
		{
			// single upper
			input:    "A",
			expected: "a",
		},
		{
			// single lower
			input:    "a",
			expected: "a",
		},
		{
			input:    "Foo",
			expected: "foo",
		},
		{
			input:    "FooBar",
			expected: "fooBar",
		},
		{
			input:    "XMLHttpRequest",
			expected: "xmlHttpRequest",
		},
		{
			input:    "AjaxXMLHttpRequest",
			expected: "ajaxXmlHttpRequest",
		},
		{
			input:    "h2w",
			expected: "h2W",
		},
		{
			input:    "Hello1World",
			expected: "hello1World",
		},
		{
			input:    "Hello11World",
			expected: "hello11World",
		},
		{
			input:    "1Hello",
			expected: "1Hello",
		},
		{
			input:    "Hello1World11foo",
			expected: "hello1World11Foo",
		},
		{
			// TODO: should this be "ids" or "iDs"?
			input:    "IDs",
			expected: "iDs",
		},
		{
			// TODO: should this be "1hello" or "1Hello"?
			input:    "1hello",
			expected: "1Hello",
		},
		{
			// TODO: should this be "b2bRegistrationRequest" or "b2BRegistrationRequest"?
			input:    "B2bRegistrationRequest",
			expected: "b2BRegistrationRequest",
		},
		{
			// TODO: should this be "b2bRegistrationRequest" or "b2BRegistrationRequest"?
			input:    "b2bRegistrationRequest",
			expected: "b2BRegistrationRequest",
		},
		{
			// TODO: should this be "b2bRegistrationB2bRequest" or "b2BRegistrationB2BRequest"?
			input:    "B2bRegistrationB2bRequest",
			expected: "b2BRegistrationB2BRequest",
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			actual := CamelCase(test.input, nil)
			if actual != test.expected {
				t.Errorf("expected %q, got %q", test.expected, actual)
			}
		})
	}
}

func TestCamelCasePreserveConsecutiveUpperCase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{
			// empty
			input:    "",
			expected: "",
		},
		{
			// single upper
			input:    "A",
			expected: "a",
		},
		{
			// single lower
			input:    "a",
			expected: "a",
		},
		{
			input:    "Foo",
			expected: "foo",
		},
		{
			input:    "FooBar",
			expected: "fooBar",
		},
		{
			input:    "FooBAR",
			expected: "fooBAR",
		},
		{
			input:    "XMLHttpRequest",
			expected: "xMLHttpRequest",
		},
		{
			input:    "AjaxXMLHttpRequest",
			expected: "ajaxXMLHttpRequest",
		},
		{
			input:    "FooIDs",
			expected: "fooIDs",
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			actual := CamelCase(test.input, &CamelCaseOptions{PreserveConsecutiveUppercase: true})
			if actual != test.expected {
				t.Errorf("expected %q, got %q", test.expected, actual)
			}
		})
	}
}
