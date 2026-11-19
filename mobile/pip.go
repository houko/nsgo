package mobile

import "github.com/nsgo-dev/nsgo/core"

// PictureInPicture renders an independent NSGo tree into iOS Picture in
// Picture. The system window takes only play, pause and skip commands;
// other input is unavailable. Background suspension retains the last
// rendered frame. Ref starts and stops it; deleting its scope stops it.
// Unsupported platforms call OnState with Supported=false. Its layer
// renderer supports views, text, images and Canvas, but not native video,
// web views, Metal shaders or backdrop effects inside the tree.
type PictureInPicture = core.PictureInPicture

// PictureInPictureState describes support, readiness, activity and errors.
type PictureInPictureState = core.PictureInPictureState
