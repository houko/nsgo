package core

// SetDevReload installs the development client's reload action for this app.
// It is used by internal/dev and is a no-op in apps that never call it.
func (cx *Scope) SetDevReload(reload func()) {
	if cx == nil || cx.rt == nil {
		return
	}
	cx.rt.devReload = reload
	if cx.rt.tree != nil {
		cx.rt.tree.bindDevReload()
		for _, surface := range cx.rt.surfaces {
			surface.bindDevReload()
		}
	}
}
