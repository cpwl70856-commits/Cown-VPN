package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// applySystemTheme follows the OS light/dark preference via Fyne's default theme.
func applySystemTheme(a fyne.App) {
	a.Settings().SetTheme(&systemTheme{})
}

// systemTheme wraps the built-in theme so Fyne continues to track OS dark mode.
type systemTheme struct{}

var _ fyne.Theme = (*systemTheme)(nil)

func (t *systemTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	return theme.DefaultTheme().Color(name, variant)
}

func (t *systemTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *systemTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *systemTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
