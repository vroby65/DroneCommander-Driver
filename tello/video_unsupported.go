//go:build android || ios || !((linux && (amd64 || arm || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64))

package tello

import (
	"errors"
	"image"
)

// VideoReceiver is a placeholder on platforms for which the embedded
// OpenH264 library is not available.
type VideoReceiver struct {
	done chan struct{}
}

func StartVideoReceiver(string, func(image.Image)) (*VideoReceiver, error) {
	return nil, errors.New("decoder H.264 non disponibile su questa piattaforma")
}

func (r *VideoReceiver) Done() <-chan struct{} { return r.done }
func (r *VideoReceiver) Err() error            { return nil }
func (r *VideoReceiver) Close() error          { return nil }
