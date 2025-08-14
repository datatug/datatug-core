package color

var (
	dangerColor  = red
	successColor = green
	warningColor = yellow
)

func Danger(s string) string {
	return Colorize(s, dangerColor)
}

func Warning(s string) string {
	return Colorize(s, warningColor)
}

func Success(s string) string {
	return Colorize(s, successColor)
}
