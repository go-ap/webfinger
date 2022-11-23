// Package webfinger
package webfinger

import "strings"

type link struct {
	Rel      string `json:"rel,omitempty"`
	Type     string `json:"type,omitempty"`
	Href     string `json:"href,omitempty"`
	Template string `json:"template,omitempty"`
}

type node struct {
	Subject string   `json:"subject"`
	Aliases []string `json:"aliases"`
	Links   []link   `json:"links"`
}

func splitResourceString(res string) (string, string) {
	split := ":"
	if strings.Contains(res, "://") {
		split = "://"
	}
	ar := strings.Split(res, split)
	if len(ar) != 2 {
		return "", ""
	}
	typ := ar[0]
	handle := ar[1]
	if handle[0] == '@' && len(handle) > 1 {
		handle = handle[1:]
	}
	return typ, handle
}
