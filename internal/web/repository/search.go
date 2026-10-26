package repository

import "strings"

// likeEscaper escapes the LIKE wildcards of user input; queries use ESCAPE '\'
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// searchPattern turns a search string into a LIKE pattern matching it
// anywhere, for "ulower(column) LIKE ? ESCAPE '\'": the input is lowercased
// like ulower() does, and % and _ in it are matched literally.
func searchPattern(s string) string {
	return "%" + likeEscaper.Replace(strings.ToLower(s)) + "%"
}
