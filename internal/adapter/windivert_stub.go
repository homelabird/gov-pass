//go:build !windows

package adapter

type WinDivertAdapter struct{}

func NewWinDivert(filter string, opts WinDivertOptions) (*WinDivertAdapter, error) {
	return nil, ErrNotImplemented
}
