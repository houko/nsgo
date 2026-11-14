//go:build linux && !android

package linux

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Linux tray icons use the freedesktop StatusNotifierItem protocol. The watcher owns the visual presentation; the app owns one item per tray ID, each on a session bus connection of its own so that it has the protocol's /StatusNotifierItem path and its bus name to itself, and receives Activate, SecondaryActivate and ContextMenu calls. Its menu is a com.canonical.dbusmenu object at /Menu on the same connection.

const (
	trayServicePrefix = "org.freedesktop.StatusNotifierItem-"
	trayItemInterface = "org.freedesktop.StatusNotifierItem"
	trayMenuInterface = "com.canonical.dbusmenu"
	trayItemPath      = "/StatusNotifierItem"
	trayMenuPath      = "/Menu"
	trayWatcher       = "org.kde.StatusNotifierWatcher"
	trayWatcherAlt    = "org.freedesktop.StatusNotifierWatcher"
	trayWatcherPath   = "/StatusNotifierWatcher"
	trayWatcherIface  = "org.kde.StatusNotifierWatcher"
	// trayMenuWait is how long, in milliseconds, a host's AboutToShow for the menu waits for the app's menu before it is answered with the one the tray has.
	trayMenuWait = 250
	// trayIconSize is the longest side of the pixmap the tray publishes.
	trayIconSize = 64
)

// GDBus answers org.freedesktop.DBus.Properties itself, from the properties declared here and the vtable's GetProperty, with an error for a property the interface lacks.
const trayIntrospection = `
<node>
  <interface name="org.freedesktop.StatusNotifierItem">
    <method name="Activate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
    <method name="SecondaryActivate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
    <method name="ContextMenu"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
    <method name="Scroll"><arg name="delta" type="i" direction="in"/><arg name="orientation" type="s" direction="in"/></method>
    <signal name="NewTitle"/><signal name="NewIcon"/><signal name="NewAttentionIcon"/><signal name="NewOverlayIcon"/><signal name="NewToolTip"/><signal name="NewStatus"><arg name="status" type="s"/></signal>
    <property name="Category" type="s" access="read"/>
    <property name="Id" type="s" access="read"/>
    <property name="Title" type="s" access="read"/>
    <property name="Status" type="s" access="read"/>
    <property name="WindowId" type="i" access="read"/>
    <property name="IconName" type="s" access="read"/>
    <property name="IconPixmap" type="a(iiay)" access="read"/>
    <property name="ToolTip" type="(sa(iiay)ss)" access="read"/>
    <property name="ItemIsMenu" type="b" access="read"/>
    <property name="Menu" type="o" access="read"/>
  </interface>
  <interface name="com.canonical.dbusmenu">
    <method name="GetLayout"><arg name="parentId" type="i" direction="in"/><arg name="recursionDepth" type="i" direction="in"/><arg name="propertyNames" type="as" direction="in"/><arg name="revision" type="u" direction="out"/><arg name="layout" type="(ia{sv}av)" direction="out"/></method>
    <method name="GetGroupProperties"><arg name="ids" type="ai" direction="in"/><arg name="propertyNames" type="as" direction="in"/><arg name="properties" type="a(ia{sv})" direction="out"/></method>
    <method name="GetProperty"><arg name="id" type="i" direction="in"/><arg name="name" type="s" direction="in"/><arg name="value" type="v" direction="out"/></method>
    <method name="Event"><arg name="id" type="i" direction="in"/><arg name="eventId" type="s" direction="in"/><arg name="data" type="v" direction="in"/><arg name="timestamp" type="u" direction="in"/></method>
    <method name="AboutToShow"><arg name="id" type="i" direction="in"/><arg name="needUpdate" type="b" direction="out"/></method>
    <method name="AboutToShowGroup"><arg name="ids" type="ai" direction="in"/><arg name="updatesNeeded" type="ai" direction="out"/><arg name="idErrors" type="ai" direction="out"/></method>
    <signal name="ItemsPropertiesUpdated"><arg name="updatedProps" type="a(ia{sv})"/><arg name="removedProps" type="a(ias)"/></signal>
    <signal name="LayoutUpdated"><arg name="revision" type="u"/><arg name="parentId" type="i"/></signal>
    <property name="Version" type="u" access="read"/>
    <property name="TextDirection" type="s" access="read"/>
    <property name="Status" type="s" access="read"/>
    <property name="IconThemePath" type="as" access="read"/>
  </interface>
</node>`

type gDBusInterfaceVTable struct {
	MethodCall  uintptr
	GetProperty uintptr
	SetProperty uintptr
	Padding     [8]uintptr
}

type linuxTray struct {
	cfg      driver.TrayConfig
	bus      uintptr // the tray's own session bus connection
	name     string
	owner    uint32
	itemID   uint32
	menuID   uint32
	menu     []driver.MenuEntry
	revision uint32
	popupID  uint32
}

var (
	gDBusNodeInfoNewForXML               func(xml *byte, err *uintptr) uintptr
	gDBusNodeInfoLookupInterface         func(info uintptr, name *byte) uintptr
	gDBusConnectionRegisterObject        func(conn uintptr, path *byte, info uintptr, vtable *gDBusInterfaceVTable, data, destroy uintptr, err *uintptr) uint32
	gDBusConnectionUnregisterObject      func(conn uintptr, registration uint32) int32
	gDBusConnectionCloseSync             func(conn, cancellable uintptr, err *uintptr) int32
	gBusOwnNameOnConnection              func(conn uintptr, name *byte, flags uint32, acquired, lost, data, destroy uintptr) uint32
	gBusUnownName                        func(owner uint32)
	gDBusMethodInvocationReturnValue     func(invocation, value uintptr)
	gDBusMethodInvocationReturnDBusError func(invocation uintptr, name, message *byte)
	gVariantNewFixedArray                func(elementType uintptr, data unsafe.Pointer, n, elementSize uintptr) uintptr
	trayBind                             = sync.OnceFunc(func() {
		bindInputSources()
		bindNotifications()
		purego.RegisterLibFunc(&gDBusNodeInfoNewForXML, gioLib, "g_dbus_node_info_new_for_xml")
		purego.RegisterLibFunc(&gDBusNodeInfoLookupInterface, gioLib, "g_dbus_node_info_lookup_interface")
		purego.RegisterLibFunc(&gDBusConnectionRegisterObject, gioLib, "g_dbus_connection_register_object")
		purego.RegisterLibFunc(&gDBusConnectionUnregisterObject, gioLib, "g_dbus_connection_unregister_object")
		purego.RegisterLibFunc(&gDBusConnectionCloseSync, gioLib, "g_dbus_connection_close_sync")
		purego.RegisterLibFunc(&gDBusAddressGetForBusSync, gioLib, "g_dbus_address_get_for_bus_sync")
		purego.RegisterLibFunc(&gBusOwnNameOnConnection, gioLib, "g_bus_own_name_on_connection")
		purego.RegisterLibFunc(&gBusUnownName, gioLib, "g_bus_unown_name")
		purego.RegisterLibFunc(&gDBusMethodInvocationReturnValue, gioLib, "g_dbus_method_invocation_return_value")
		purego.RegisterLibFunc(&gDBusMethodInvocationReturnDBusError, gioLib, "g_dbus_method_invocation_return_dbus_error")
		purego.RegisterLibFunc(&gVariantNewFixedArray, glibLib, "g_variant_new_fixed_array")
		purego.RegisterLibFunc(&gVariantNewObjectPath, glibLib, "g_variant_new_object_path")
	})
	trayNodeInfo      uintptr
	trayVTable        = gDBusInterfaceVTable{}
	cbTrayMethod      uintptr
	cbTrayGetProperty uintptr
	cbTraySetProperty uintptr
	cbTrayAcquired    uintptr
)

func init() {
	cbTrayMethod = newCallback(trayMethodCall)
	cbTrayGetProperty = newCallback(trayGetProperty)
	cbTraySetProperty = newCallback(traySetProperty)
	cbTrayAcquired = newCallback(trayNameAcquired)
	trayVTable = gDBusInterfaceVTable{MethodCall: cbTrayMethod, GetProperty: cbTrayGetProperty, SetProperty: cbTraySetProperty}
}

func (w *window) applyTray(cfg driver.TrayConfig) {
	trayBind()
	tr := w.trays[cfg.ID]
	if cfg.Removed {
		if tr != nil {
			w.removeTray(tr)
		}
		return
	}
	if tr != nil {
		tr.cfg = cfg
		return
	}
	if trayNodeInfo == 0 {
		var err uintptr
		trayNodeInfo = gDBusNodeInfoNewForXML(cstr(trayIntrospection), &err)
		if err != 0 {
			gErrorFree(err)
			trayNodeInfo = 0
		}
	}
	if trayNodeInfo == 0 {
		return
	}
	itemInfo := gDBusNodeInfoLookupInterface(trayNodeInfo, cstr(trayItemInterface))
	menuInfo := gDBusNodeInfoLookupInterface(trayNodeInfo, cstr(trayMenuInterface))
	if itemInfo == 0 || menuInfo == 0 {
		return
	}
	bus := trayConnection()
	if bus == 0 {
		return
	}
	tr = &linuxTray{cfg: cfg, bus: bus, name: fmt.Sprintf("%s%d-%d", trayServicePrefix, os.Getpid(), cfg.ID)}
	var err uintptr
	tr.itemID = gDBusConnectionRegisterObject(bus, cstr(trayItemPath), itemInfo, &trayVTable, w.handle, 0, &err)
	if err != 0 {
		gErrorFree(err)
		err = 0
	}
	if tr.itemID == 0 {
		closeTrayConnection(bus)
		return
	}
	tr.menuID = gDBusConnectionRegisterObject(bus, cstr(trayMenuPath), menuInfo, &trayVTable, w.handle, 0, &err)
	if err != 0 {
		gErrorFree(err)
	}
	w.trays[cfg.ID] = tr
	tr.owner = gBusOwnNameOnConnection(bus, cstr(tr.name), 0, cbTrayAcquired, 0, w.handle, 0)
}

// trayConnection opens a session bus connection for one tray, 0 without a session bus.
func trayConnection() uintptr {
	const busSession = 2
	const authClient, messageBus = 1 << 0, 1 << 3
	var err uintptr
	address := gDBusAddressGetForBusSync(busSession, 0, &err)
	if err != 0 {
		gErrorFree(err)
		return 0
	}
	bus := gDBusConnectionNewForAddressSync(cstr(goString(address)), authClient|messageBus, 0, 0, &err)
	gFree(address)
	if err != 0 {
		gErrorFree(err)
		return 0
	}
	return bus
}

// closeTrayConnection closes a tray's connection, which takes its bus name and objects away and so has the watcher drop the item.
func closeTrayConnection(bus uintptr) {
	var err uintptr
	gDBusConnectionCloseSync(bus, 0, &err)
	if err != 0 {
		gErrorFree(err)
	}
	gObjectUnref(bus)
}

func (w *window) removeTray(tr *linuxTray) {
	if tr.menuID != 0 {
		gDBusConnectionUnregisterObject(tr.bus, tr.menuID)
	}
	if tr.itemID != 0 {
		gDBusConnectionUnregisterObject(tr.bus, tr.itemID)
	}
	if tr.owner != 0 {
		gBusUnownName(tr.owner)
	}
	closeTrayConnection(tr.bus)
	delete(w.trays, tr.cfg.ID)
}

func (w *window) removeTrays() {
	for _, tr := range w.trays {
		w.removeTray(tr)
	}
}

func trayNameAcquired(bus, name, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing || w.trayOn(bus) == nil {
		return
	}
	// The watcher finds the item at /StatusNotifierItem of the service it names. A missing watcher is normal on GNOME without its AppIndicator extension.
	service := goString(name)
	if reply := dbusCall(bus, trayWatcher, trayWatcherPath, trayWatcherIface, "RegisterStatusNotifierItem", gVariantNewString(cstr(service))); reply != 0 {
		gVariantUnref(reply)
	} else {
		gVariantUnrefNonZero(dbusCall(bus, trayWatcherAlt, trayWatcherPath, trayWatcherAlt, "RegisterStatusNotifierItem", gVariantNewString(cstr(service))))
	}
}

// trayOn returns the tray whose connection bus is, nil for none.
func (w *window) trayOn(bus uintptr) *linuxTray {
	for _, tr := range w.trays {
		if tr.bus == bus {
			return tr
		}
	}
	return nil
}

func trayMethodCall(conn, sender, path, iface, method, params, invocation, data uintptr) {
	w := windowOf(data)
	var tr *linuxTray
	if w != nil && w.engine != nil && !w.closing {
		tr = w.trayOn(conn)
	}
	if tr == nil {
		trayError(invocation, "org.freedesktop.DBus.Error.UnknownObject", "no such tray")
		return
	}
	if goString(iface) == trayMenuInterface {
		gDBusMethodInvocationReturnValue(invocation, trayMenuMethod(w, tr, goString(method), params))
		return
	}
	switch goString(method) {
	case "Activate":
		w.engine.Dispatch(driver.Event{Kind: driver.EventTray, Count: tr.cfg.ID, Index: 0})
	case "SecondaryActivate", "ContextMenu":
		w.engine.Dispatch(driver.Event{Kind: driver.EventTray, Count: tr.cfg.ID, Index: 1})
	}
	returnTray(invocation)
}

func trayGetProperty(conn, sender, path, iface, property uintptr, err, data uintptr) uintptr {
	var tr *linuxTray
	id := ""
	if w := windowOf(data); w != nil {
		tr, id = w.trayOn(conn), w.opts.ID
	}
	if tr == nil {
		// GDBus asks only for declared properties and needs a value or an error for each.
		tr = &linuxTray{}
	}
	if goString(iface) == trayMenuInterface {
		return menuProperty(goString(property))
	}
	return trayProperty(tr, id, goString(property))
}

func traySetProperty(conn, sender, path, iface, property, value, err, data uintptr) uintptr {
	return 0
}

func returnTray(invocation uintptr) {
	if invocation != 0 {
		gDBusMethodInvocationReturnValue(invocation, gVariantNewTuple(nil, 0))
	}
}

func trayError(invocation uintptr, name, message string) {
	if invocation != 0 {
		gDBusMethodInvocationReturnDBusError(invocation, cstr(name), cstr(message))
	}
}

// trayProperty is a declared StatusNotifierItem property of tr, appID naming the app.
func trayProperty(tr *linuxTray, appID string, property string) uintptr {
	cfg := tr.cfg
	switch property {
	case "Category":
		return gVariantNewString(cstr("ApplicationStatus"))
	case "Id":
		return gVariantNewString(cstr(appID))
	case "Title":
		return gVariantNewString(cstr(cfg.Title))
	case "Status":
		return gVariantNewString(cstr("Active"))
	case "WindowId":
		return gVariantNewInt32(0)
	case "IconName":
		return gVariantNewString(cstr(appID))
	case "IconPixmap":
		return trayPixmap(cfg.Icon)
	case "ToolTip":
		children := []uintptr{gVariantNewString(cstr(appID)), trayPixmap(cfg.Icon), gVariantNewString(cstr(cfg.Tooltip)), gVariantNewString(cstr(""))}
		return gVariantNewTuple(&children[0], uintptr(len(children)))
	case "ItemIsMenu":
		return gVariantNewBoolean(0)
	case "Menu":
		return gVariantNewObjectPath(cstr(trayMenuPath))
	}
	return gVariantNewString(cstr(""))
}

// menuProperty is a declared property of the dbusmenu object.
func menuProperty(name string) uintptr {
	switch name {
	case "Version":
		return gVariantNewUint32(3)
	case "TextDirection":
		return gVariantNewString(cstr("ltr"))
	case "Status":
		return gVariantNewString(cstr("normal"))
	case "IconThemePath":
		return menuArray("s", nil)
	}
	return gVariantNewString(cstr(""))
}

func trayMenuMethod(w *window, tr *linuxTray, method string, params uintptr) uintptr {
	switch method {
	case "GetLayout":
		// The method's output is (u(ia{sv}av)).
		children := []uintptr{gVariantNewUint32(tr.revision), menuNode(0, nil, tr.menu)}
		return gVariantNewTuple(&children[0], uintptr(len(children)))
	case "GetGroupProperties":
		return menuGroupProperties(tr.menu)
	case "GetProperty":
		name := variantString(params, 1)
		value := menuProperty(name)
		v := gVariantNewVariant(value)
		return gVariantNewTuple(&v, 1)
	case "Event":
		if variantString(params, 1) == "clicked" {
			child := gVariantGetChildValue(params, 0)
			itemID := uint32(gVariantGetInt32(child))
			gVariantUnref(child)
			if itemID != 0 {
				w.engine.Dispatch(driver.Event{Kind: driver.EventMenu, Count: itemID})
			}
			if tr.popupID != 0 {
				w.engine.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: tr.popupID})
				tr.popupID = 0
			}
		}
		return gVariantNewTuple(nil, 0)
	case "AboutToShow":
		v := gVariantNewBoolean(1)
		return gVariantNewTuple(&v, 1)
	case "AboutToShowGroup":
		return menuEmptyIntArrays()
	}
	return gVariantNewTuple(nil, 0)
}

func menuNode(id int32, entry *driver.MenuEntry, children []driver.MenuEntry) uintptr {
	properties := menuEntryProperties(entry)
	childValues := make([]uintptr, len(children))
	for i := range children {
		childValues[i] = gVariantNewVariant(menuNode(int32(children[i].ID), &children[i], children[i].Items))
	}
	childArray := menuArray("v", childValues)
	node := []uintptr{gVariantNewInt32(id), properties, childArray}
	return gVariantNewTuple(&node[0], uintptr(len(node)))
}

func menuEntryProperties(entry *driver.MenuEntry) uintptr {
	props := []uintptr{}
	if entry != nil {
		props = append(props, menuDictEntry("label", gVariantNewString(cstr(entry.Label))))
		props = append(props, menuDictEntry("enabled", gVariantNewBoolean(menuBool(!entry.Disabled))))
		props = append(props, menuDictEntry("visible", gVariantNewBoolean(1)))
		if entry.Separator {
			props = append(props, menuDictEntry("type", gVariantNewString(cstr("separator"))))
		}
		if len(entry.Items) != 0 {
			props = append(props, menuDictEntry("children-display", gVariantNewString(cstr("submenu"))))
		}
		if entry.Checkable {
			props = append(props, menuDictEntry("toggle-type", gVariantNewString(cstr("checkmark"))))
			state := int32(0)
			if entry.Checked {
				state = 1
			}
			props = append(props, menuDictEntry("toggle-state", gVariantNewInt32(state)))
		}
	}
	return menuDict(props)
}

func menuGroupProperties(entries []driver.MenuEntry) uintptr {
	values := make([]uintptr, len(entries))
	for i := range entries {
		children := []uintptr{gVariantNewInt32(int32(entries[i].ID)), menuEntryProperties(&entries[i])}
		values[i] = gVariantNewTuple(&children[0], uintptr(len(children)))
	}
	return menuArray("(ia{sv})", values)
}

func menuEmptyIntArrays() uintptr {
	empty := menuArray("i", nil)
	values := []uintptr{empty, menuArray("i", nil)}
	return gVariantNewTuple(&values[0], uintptr(len(values)))
}

func menuDictEntry(key string, value uintptr) uintptr {
	return gVariantNewDictEntry(gVariantNewString(cstr(key)), gVariantNewVariant(value))
}

func menuDict(entries []uintptr) uintptr {
	return menuArray("{sv}", entries)
}

func menuArray(signature string, values []uintptr) uintptr {
	t := gVariantTypeNew(cstr(signature))
	var ptr *uintptr
	if len(values) != 0 {
		ptr = &values[0]
	}
	v := gVariantNewArray(t, ptr, uintptr(len(values)))
	gVariantTypeFree(t)
	return v
}

func menuBool(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

// trayPixmap converts the PNG in TrayConfig to the StatusNotifierItem's
// network-order ARGB32 pixmap. An empty array lets a host use IconName.
func trayPixmap(data []byte) uintptr {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Empty() {
		t := gVariantTypeNew(cstr("(iiay)"))
		v := gVariantNewArray(t, nil, 0)
		gVariantTypeFree(t)
		return v
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > 64 || h > 64 {
		w, h = min(w, 64), min(h, 64)
	}
	pix := make([]byte, w*h*4)
	for y := range h {
		sy := b.Min.Y + y*b.Dy()/h
		for x := range w {
			sx := b.Min.X + x*b.Dx()/w
			r, g, bl, a := img.At(sx, sy).RGBA()
			i := (y*w + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = byte(a/0x100), byte(r/0x100), byte(g/0x100), byte(bl/0x100)
		}
	}
	byteType := gVariantTypeNew(cstr("y"))
	bytesValue := gVariantNewFixedArray(byteType, unsafe.Pointer(&pix[0]), uintptr(len(pix)), 1)
	gVariantTypeFree(byteType)
	children := []uintptr{gVariantNewInt32(int32(w)), gVariantNewInt32(int32(h)), bytesValue}
	item := gVariantNewTuple(&children[0], uintptr(len(children)))
	tupleType := gVariantTypeNew(cstr("(iiay)"))
	result := gVariantNewArray(tupleType, &item, 1)
	gVariantTypeFree(tupleType)
	return result
}
