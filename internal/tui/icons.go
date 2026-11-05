package tui

// Terminal-safe glyphs only (text presentation, single cell, no emoji):
// bot, square, dot, angle brackets, energy bolt for full-access mode.
const (
	IconBot    = "◉" // big bot — assistant / agent
	IconSquare = "■"
	IconSqOpen = "□"
	IconDot    = "•"
	IconLDot   = "·"
	IconLeft   = "<"
	IconRight  = ">"
	IconPrompt = "›"
	IconArrow  = "▸"
	IconOK     = "✔"
	IconErr    = "✖"
	IconWarn   = "▲"
	IconTool   = "●"
	IconBar    = "─"
	IconEnergy = "ϟ" // full-access / full-auto permission mode
	IconSpark  = "✦"
)

// SpinFrames is a braille spinner (single-cell, terminal-safe).
var SpinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ModeIcon maps a permission mode to its icon; full-auto gets the energy bolt.
func ModeIcon(mode string) string {
	switch mode {
	case "full-auto":
		return IconEnergy
	case "auto-edit":
		return IconSquare
	case "plan":
		return IconSqOpen
	default:
		return IconDot
	}
}
