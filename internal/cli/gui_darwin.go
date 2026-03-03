//go:build desktop && darwin

package cli

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

void setupMacOSMenu() {
	NSMenu *menubar = [[NSMenu alloc] init];

	// App menu
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

	// Edit menu — items without key equivalents (clipboard handled by event monitor)
	NSMenuItem *editMenuItem = [[NSMenuItem alloc] init];
	NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
	[editMenu addItemWithTitle:@"Undo"       action:@selector(undo:)      keyEquivalent:@""];
	[editMenu addItemWithTitle:@"Redo"       action:@selector(redo:)      keyEquivalent:@""];
	[editMenu addItem:[NSMenuItem separatorItem]];
	[editMenu addItemWithTitle:@"Cut"        action:@selector(cut:)       keyEquivalent:@""];
	[editMenu addItemWithTitle:@"Copy"       action:@selector(copy:)      keyEquivalent:@""];
	[editMenu addItemWithTitle:@"Paste"      action:@selector(paste:)     keyEquivalent:@""];
	[editMenu addItemWithTitle:@"Delete"     action:@selector(delete:)    keyEquivalent:@""];
	[editMenu addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@""];
	[editMenuItem setSubmenu:editMenu];
	[menubar addItem:editMenuItem];

	[NSApp setMainMenu:menubar];
}

// installClipboardMonitor intercepts Cmd+C/V/X/A at the NSEvent level,
// bypassing the broken menu/responder chain for WKWebView clipboard.
void installClipboardMonitor(void *windowPtr) {
	NSWindow *window = (__bridge NSWindow *)windowPtr;

	[NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown handler:^NSEvent *(NSEvent *event) {
		if (!([event modifierFlags] & NSEventModifierFlagCommand)) return event;

		NSString *chars = [event charactersIgnoringModifiers];
		if (!chars.length) return event;
		unichar ch = [chars characterAtIndex:0];

		WKWebView *wv = (WKWebView *)[window contentView];
		if (!wv || ![wv isKindOfClass:[WKWebView class]]) return event;

		if (ch == 'c' || ch == 'x') {
			[wv evaluateJavaScript:@"window.getSelection().toString()"
				 completionHandler:^(id result, NSError *error) {
				if (result && [result isKindOfClass:[NSString class]] && [result length] > 0) {
					NSPasteboard *pb = [NSPasteboard generalPasteboard];
					[pb clearContents];
					[pb setString:result forType:NSPasteboardTypeString];
					if (ch == 'x') {
						dispatch_async(dispatch_get_main_queue(), ^{
							[wv evaluateJavaScript:@"document.execCommand('delete')" completionHandler:nil];
						});
					}
				}
			}];
			return nil;
		}

		if (ch == 'v') {
			NSString *text = [[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeString];
			if (text) {
				// JSON-encode to safely embed in JS string
				NSData *jsonData = [NSJSONSerialization dataWithJSONObject:@[text] options:0 error:nil];
				NSString *jsonArray = [[NSString alloc] initWithData:jsonData encoding:NSUTF8StringEncoding];
				// jsonArray is like ["text"], extract just the string part
				NSString *js = [NSString stringWithFormat:
					@"(function(){var t=%@[0];"
					 "var el=document.activeElement;"
					 "if(el&&(el.tagName==='INPUT'||el.tagName==='TEXTAREA'||el.isContentEditable)){"
					 "document.execCommand('insertText',false,t);"
					 "}else{"
					 "var ta=document.createElement('textarea');"
					 "ta.value=t;ta.style.position='fixed';ta.style.opacity='0';"
					 "document.body.appendChild(ta);ta.select();"
					 "document.execCommand('copy');document.body.removeChild(ta);"
					 "}"
					 "})()", jsonArray];
				[wv evaluateJavaScript:js completionHandler:nil];
			}
			return nil;
		}

		if (ch == 'a') {
			[wv evaluateJavaScript:@"document.execCommand('selectAll')" completionHandler:nil];
			return nil;
		}

		return event;
	}];
}
*/
import "C"

import "unsafe"

func setupNativeMenu() {
	C.setupMacOSMenu()
}

func installClipboardMonitor(windowPtr unsafe.Pointer) {
	C.installClipboardMonitor(windowPtr)
}
