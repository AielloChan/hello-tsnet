#import <AppKit/AppKit.h>
#import "menu.h"

extern void goMenuQuit(void);

@interface HTAppDelegate : NSObject <NSApplicationDelegate>
@property(nonatomic, strong) NSStatusItem *statusItem;
@property(nonatomic, strong) NSURL *controlURL;
@property(nonatomic) BOOL quitting;
@property(nonatomic) BOOL systemTermination;
@end

static void showError(NSString *message) {
    [NSApp activateIgnoringOtherApps:YES];
    NSAlert *alert = [NSAlert new];
    alert.messageText = @"hello-tsnet 无法继续运行";
    alert.informativeText = message;
    alert.alertStyle = NSAlertStyleWarning;
    [alert addButtonWithTitle:@"好"];
    [alert runModal];
}

@implementation HTAppDelegate
- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    self.statusItem = [NSStatusBar.systemStatusBar statusItemWithLength:NSSquareStatusItemLength];
    NSString *iconPath = [NSBundle.mainBundle pathForResource:@"MenuIcon" ofType:@"png"];
    NSImage *icon = iconPath ? [[NSImage alloc] initWithContentsOfFile:iconPath] : nil;
    if (!icon) {
        // Also keep an identifiable icon when running the executable outside its bundle.
        icon = [NSImage imageWithSystemSymbolName:@"circle.grid.3x3.fill" accessibilityDescription:@"hello-tsnet"];
    }
    icon.size = NSMakeSize(20, 20);
    icon.template = NO;
    self.statusItem.button.image = icon;
    self.statusItem.button.toolTip = @"hello-tsnet";
    [self.statusItem.button setAccessibilityLabel:@"hello-tsnet"];
    NSMenu *menu = [[NSMenu alloc] initWithTitle:@"hello-tsnet"];
    menu.autoenablesItems = NO;
    NSArray *titles = @[@"关于", @"控制面板", @"退出应用"];
    SEL actions[] = {@selector(about:), @selector(controlPanel:), @selector(quit:)};
    for (NSUInteger index = 0; index < titles.count; index++) {
        NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:titles[index] action:actions[index] keyEquivalent:@""];
        item.target = self;
        [menu addItem:item];
    }
    self.statusItem.menu = menu;
}

- (void)about:(id)sender {
    [NSApp activateIgnoringOtherApps:YES];
    NSString *version = [NSBundle.mainBundle objectForInfoDictionaryKey:@"CFBundleShortVersionString"] ?: @"开发版";
    [NSApp orderFrontStandardAboutPanelWithOptions:@{
        NSAboutPanelOptionApplicationName: @"hello-tsnet",
        NSAboutPanelOptionApplicationVersion: version,
        NSAboutPanelOptionVersion: @"",
        NSAboutPanelOptionCredits: [[NSAttributedString alloc] initWithString:@"轻量的本机 TCP → Tailscale 转发网关\n\n独立应用，基于官方 tsnet 构建。"]
    }];
}

- (void)controlPanel:(id)sender {
    if (![NSWorkspace.sharedWorkspace openURL:self.controlURL]) {
        showError(@"无法打开默认浏览器。请在浏览器中访问控制面板地址。");
    }
}

- (void)quit:(id)sender {
    if (self.quitting) { return; }
    self.quitting = YES;
    for (NSMenuItem *item in self.statusItem.menu.itemArray) { item.enabled = NO; }
    self.statusItem.button.toolTip = @"hello-tsnet 正在退出…";
    goMenuQuit();
}

- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
    self.systemTermination = YES;
    [self quit:nil];
    return NSTerminateLater;
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender { return NO; }
@end

void HTMenuRun(const char *controlURL) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        HTAppDelegate *delegate = [HTAppDelegate new];
        delegate.controlURL = [NSURL URLWithString:[NSString stringWithUTF8String:controlURL]];
        NSApp.delegate = delegate;
        [NSApp run];
        [NSStatusBar.systemStatusBar removeStatusItem:delegate.statusItem];
        NSApp.delegate = nil;
    }
}

void HTMenuFinish(const char *errorMessage) {
    @autoreleasepool {
        NSString *message = [NSString stringWithUTF8String:errorMessage];
        dispatch_async(dispatch_get_main_queue(), ^{
            if (message.length > 0) { showError(message); }
            HTAppDelegate *delegate = (HTAppDelegate *)NSApp.delegate;
            if (delegate.systemTermination) {
                [NSApp replyToApplicationShouldTerminate:YES];
            } else {
                [NSApp stop:nil];
                // Wake the event loop so Run can return and Go can release its lock.
                [NSApp postEvent:[NSEvent otherEventWithType:NSEventTypeApplicationDefined
                    location:NSZeroPoint modifierFlags:0 timestamp:0 windowNumber:0
                    context:nil subtype:0 data1:0 data2:0] atStart:NO];
            }
        });
    }
}

void HTShowError(const char *errorMessage) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        showError([NSString stringWithUTF8String:errorMessage]);
    }
}
