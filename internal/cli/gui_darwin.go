//go:build desktop && darwin

package cli

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

void setupMacOSMenu() {
	// Must run on the main thread — called before webview.Run() so we are on it.
	NSMenu *menubar = [[NSMenu alloc] init];

	// App menu (first item is always the application menu)
	NSMenuItem *appMenuItem = [[NSMenuItem alloc] init];
	NSMenu *appMenu = [[NSMenu alloc] init];
	[appMenu addItemWithTitle:@"Hide"
					   action:@selector(hide:)
				keyEquivalent:@"h"];
	[appMenu addItemWithTitle:@"Hide Others"
					   action:@selector(hideOtherApplications:)
				keyEquivalent:@""];
	[appMenu addItem:[NSMenuItem separatorItem]];
	[appMenu addItemWithTitle:@"Quit"
					   action:@selector(terminate:)
				keyEquivalent:@"q"];
	[appMenuItem setSubmenu:appMenu];
	[menubar addItem:appMenuItem];

	// Edit menu — provides standard selectors that WKWebView responder chain handles
	NSMenuItem *editMenuItem = [[NSMenuItem alloc] init];
	NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
	[editMenu addItemWithTitle:@"Undo"   action:@selector(undo:)     keyEquivalent:@"z"];
	[editMenu addItemWithTitle:@"Redo"   action:@selector(redo:)     keyEquivalent:@"Z"];
	[editMenu addItem:[NSMenuItem separatorItem]];
	[editMenu addItemWithTitle:@"Cut"    action:@selector(cut:)      keyEquivalent:@"x"];
	[editMenu addItemWithTitle:@"Copy"   action:@selector(copy:)     keyEquivalent:@"c"];
	[editMenu addItemWithTitle:@"Paste"  action:@selector(paste:)    keyEquivalent:@"v"];
	[editMenu addItemWithTitle:@"Delete" action:@selector(delete:)   keyEquivalent:@""];

	NSMenuItem *selectAllItem = [editMenu addItemWithTitle:@"Select All"
													action:@selector(selectAll:)
											 keyEquivalent:@"a"];
	(void)selectAllItem;

	[editMenuItem setSubmenu:editMenu];
	[menubar addItem:editMenuItem];

	[NSApp setMainMenu:menubar];
}
*/
import "C"

func setupNativeMenu() {
	C.setupMacOSMenu()
}
