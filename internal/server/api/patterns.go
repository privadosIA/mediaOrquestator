package api

import "regexp"

// ProjectIDPattern is what a project id looks like.
var ProjectIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// LanguagePattern is an ISO 639-1 (or 639-2) language code such as es or en.
var LanguagePattern = regexp.MustCompile(`^[a-z]{2,3}$`)
