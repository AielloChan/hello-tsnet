#import <AppKit/AppKit.h>
#import <objc/runtime.h>
#import "../internal/menubar/menu_darwin.m"

static int quitCount = 0;
void goMenuQuit(void) { quitCount++; }

static NSURL *openedURL;
@interface NSWorkspace (MenuTests)
- (BOOL)testOpenURL:(NSURL *)url;
@end
@implementation NSWorkspace (MenuTests)
- (BOOL)testOpenURL:(NSURL *)url { openedURL = url; return YES; }
@end

static BOOL containsApplicationName(NSView *view) {
    if ([view isKindOfClass:NSTextField.class] &&
        [((NSTextField *)view).stringValue isEqualToString:@"Hello TSNet"]) { return YES; }
    for (NSView *child in view.subviews) {
        if (containsApplicationName(child)) { return YES; }
    }
    return NO;
}

int main(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        [NSApp finishLaunching];
        HTAppDelegate *delegate = [HTAppDelegate new];
        delegate.controlURL = [NSURL URLWithString:@"http://127.0.0.1:6060"];
        [delegate applicationDidFinishLaunching:[NSNotification notificationWithName:NSApplicationDidFinishLaunchingNotification object:NSApp]];
        NSMenu *menu = delegate.statusItem.menu;
        NSArray *titles = [menu.itemArray valueForKey:@"title"];
        if (![titles isEqualToArray:@[@"关于", @"控制面板", @"退出应用"]]) {
            NSLog(@"Unexpected menu: %@", titles);
            return 1;
        }
        if (delegate.statusItem.button.image == nil || delegate.statusItem.button.image.isTemplate) {
            NSLog(@"Missing color menu icon");
            return 1;
        }
        SEL actions[] = {@selector(about:), @selector(controlPanel:), @selector(quit:)};
        for (NSUInteger index = 0; index < 3; index++) {
            NSMenuItem *item = menu.itemArray[index];
            if (item.target != delegate || item.action != actions[index]) {
                NSLog(@"Menu action is not wired correctly: %@", item.title);
                return 1;
            }
        }
        [menu performActionForItemAtIndex:0];
        NSWindow *aboutWindow = nil;
        for (NSWindow *window in NSApp.windows) {
            if (window.isVisible && containsApplicationName(window.contentView)) { aboutWindow = window; }
        }
        if (!aboutWindow) {
            NSLog(@"About menu did not display the application name");
            return 1;
        }
        [aboutWindow performClose:nil];
        if (quitCount != 0 || [delegate applicationShouldTerminateAfterLastWindowClosed:NSApp]) {
            NSLog(@"Closing About must leave the gateway running");
            return 1;
        }
        Method openURL = class_getInstanceMethod(NSWorkspace.class, @selector(openURL:));
        Method captureURL = class_getInstanceMethod(NSWorkspace.class, @selector(testOpenURL:));
        method_exchangeImplementations(openURL, captureURL);
        [menu performActionForItemAtIndex:1];
        method_exchangeImplementations(openURL, captureURL);
        if (![openedURL.absoluteString isEqualToString:@"http://127.0.0.1:6060"]) {
            NSLog(@"Control panel did not open the configured loopback URL");
            return 1;
        }
        [menu performActionForItemAtIndex:2];
        [delegate quit:nil];
        if (quitCount != 1 || menu.itemArray.lastObject.enabled) {
            NSLog(@"Quit must cancel once and disable the menu");
            return 1;
        }
        [NSStatusBar.systemStatusBar removeStatusItem:delegate.statusItem];
        puts("Native menu: action wiring, color icon, About open/close, control panel URL, idempotent quit passed");
    }
    return 0;
}
