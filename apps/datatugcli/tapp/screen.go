package tapp

import "github.com/rivo/tview"

type Screen interface {
	tview.Primitive
	Options() ScreenOptions
	//Window() tview.Primitive
	Activate() error
	Close() error
}

type ScreenOptions struct {
	fullScreen bool
}

func (o ScreenOptions) FullScreen() bool {
	return o.fullScreen
}

func FullScreen() ScreenOptions {
	return ScreenOptions{fullScreen: true}
}
