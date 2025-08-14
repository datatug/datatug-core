package color

import "fmt"

type Name string

const (
	red    Name = "\x1b[91m"
	green  Name = "\x1b[32m"
	blue   Name = "\x1b[94m"
	gray   Name = "\x1b[90m"
	yellow Name = "\x1b[33m"
)

var defaultColor Name = "\x1b[39m"

func SetDefaultColor(color Name) {
	defaultColor = color
}

func Colorize(s string, color Name) string {
	return fmt.Sprintf("%s%s%s", color, s, defaultColor)
}

func Red(s string) string {
	return Colorize(s, red)
}

func Green(s string) string {
	return Colorize(s, green)
}

func Blue(s string) string {
	return Colorize(s, blue)
}

func Gray(s string) string {
	return Colorize(s, gray)
}

func Yellow(string2 string) string {
	return Colorize(string2, yellow)
}
