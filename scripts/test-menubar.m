#import <AppKit/AppKit.h>
#import <objc/runtime.h>
#import "../internal/menubar/menu_darwin.m"

static int quitCount = 0;
void goMenuQuit(void) { quitCount++; }
void goMenuRefresh(void) {}
static unsigned long long probeSession;
static BOOL probing;
unsigned long long goMenuSetProbing(int active) {
    if (active && !probing) { probeSession++; }
    probing = active != 0;
    return probing ? probeSession : 0;
}
static BOOL routeHelperNeeded;
static BOOL routeHelperUpgrade;
static int installCount, uninstallCount;
int goMenuRouteHelperNeeded(void) { return routeHelperNeeded; }
int goMenuRouteHelperNeedsUpgrade(void) { return routeHelperUpgrade; }
void goMenuInstallRouteHelper(void) { installCount++; }
void goMenuUninstallRouteHelper(void) { uninstallCount++; }
static int signedIn = 1;
static int iconState = HTMenuIconLoading;
int goMenuIconState(void) { return iconState; }
static int authorizeCount, logoutCount;
void goMenuAuthorize(void) { authorizeCount++; }
void goMenuLogout(void) { logoutCount++; }
int goMenuSignedIn(void) { return signedIn; }

static NSPasteboard *testPasteboard;
@interface NSPasteboard (MenuTests)
+ (NSPasteboard *)testGeneralPasteboard;
@end
@implementation NSPasteboard (MenuTests)
+ (NSPasteboard *)testGeneralPasteboard { return testPasteboard; }
@end

static NSModalResponse routeHelperResponse;
@interface NSAlert (RouteHelperTests)
- (NSModalResponse)testRouteHelperResponse;
@end
@implementation NSAlert (RouteHelperTests)
- (NSModalResponse)testRouteHelperResponse { return routeHelperResponse; }
@end

static NSURL *openedURL;
@interface NSWorkspace (MenuTests)
- (BOOL)testOpenURL:(NSURL *)url;
@end
@implementation NSWorkspace (MenuTests)
- (BOOL)testOpenURL:(NSURL *)url { openedURL = url; return YES; }
@end

static BOOL containsLabelText(NSView *view, NSString *text) {
    if ([view isKindOfClass:NSTextField.class] &&
        [((NSTextField *)view).stringValue isEqualToString:text]) { return YES; }
    for (NSView *child in view.subviews) {
        if (containsLabelText(child, text)) { return YES; }
    }
    return NO;
}

static BOOL containsLink(NSView *view, NSString *target) {
    if ([view isKindOfClass:NSTextView.class]) {
        NSAttributedString *credits = ((NSTextView *)view).textStorage;
        for (NSUInteger index = 0; index < credits.length; index++) {
            if ([[[credits attribute:NSLinkAttributeName atIndex:index effectiveRange:nil] description] isEqualToString:target]) {
                return YES;
            }
        }
    }
    for (NSView *child in view.subviews) {
        if (containsLink(child, target)) { return YES; }
    }
    return NO;
}

static NSButton *findButton(NSView *view, NSString *title) {
    if ([view isKindOfClass:NSButton.class] &&
        [((NSButton *)view).title isEqualToString:title]) { return (NSButton *)view; }
    for (NSView *child in view.subviews) {
        NSButton *button = findButton(child, title);
        if (button) { return button; }
    }
    return nil;
}

static CGFloat horizontalInkWidth(NSImage *image) {
    NSBitmapImageRep *bitmap = [NSBitmapImageRep imageRepWithData:image.TIFFRepresentation];
    NSInteger left = bitmap.pixelsWide;
    NSInteger right = -1;
    for (NSInteger y = 0; y < bitmap.pixelsHigh; y++) {
        for (NSInteger x = 0; x < bitmap.pixelsWide; x++) {
            if ([bitmap colorAtX:x y:y].alphaComponent > 0.1) {
                left = MIN(left, x);
                right = MAX(right, x);
            }
        }
    }
    return right < left ? 0 : (right - left + 1) * image.size.width / bitmap.pixelsWide;
}

static NSColor *dotColor(NSBitmapImageRep *bitmap, NSUInteger row, NSUInteger column) {
    NSInteger x = (4 + column * 6) * bitmap.pixelsWide / 20;
    NSInteger y = (4 + row * 6) * bitmap.pixelsHigh / 20;
    return [[bitmap colorAtX:x y:y] colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
}

static NSUInteger litDots(NSImage *image) {
    NSBitmapImageRep *bitmap = [NSBitmapImageRep imageRepWithData:image.TIFFRepresentation];
    NSUInteger mask = 0;
    for (NSUInteger row = 0; row < 3; row++) {
        for (NSUInteger col = 0; col < 3; col++) {
            NSColor *color = dotColor(bitmap, row, col);
            if (color.redComponent - color.greenComponent > 0.15) { mask |= 1u << (row * 3 + col); }
        }
    }
    return mask;
}

int main(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        [NSApp finishLaunching];
        HTAppDelegate *delegate = [HTAppDelegate new];
        [delegate applicationDidFinishLaunching:[NSNotification notificationWithName:NSApplicationDidFinishLaunchingNotification object:NSApp]];
        NSMenu *menu = delegate.statusItem.menu;
        NSArray *titles = [menu.itemArray valueForKey:@"title"];
        if (![titles isEqualToArray:@[@"关于", @"控制面板", @"退出应用"]]) {
            NSLog(@"Unexpected menu: %@", titles);
            return 1;
        }
        if (delegate.statusItem.button.image == nil || delegate.statusItem.button.image.isTemplate) {
            NSLog(@"The menu icon must render its own pink color");
            return 1;
        }
        NSImage *lightIcon = menuIcon(0, [NSAppearance appearanceNamed:NSAppearanceNameAqua]);
        NSImage *darkIcon = menuIcon(0, [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua]);
        NSColor *lightDot = dotColor([NSBitmapImageRep imageRepWithData:lightIcon.TIFFRepresentation], 0, 0);
        NSColor *darkDot = dotColor([NSBitmapImageRep imageRepWithData:darkIcon.TIFFRepresentation], 0, 0);
        if (lightDot.alphaComponent < 0.9 || lightDot.redComponent > 0.15 ||
            darkDot.alphaComponent < 0.9 || darkDot.greenComponent < 0.85) {
            NSLog(@"Unlit dots must be black in light mode and white in dark mode");
            return 1;
        }
        NSUInteger firstFrame = litDots(delegate.statusItem.button.image);
        [delegate refreshStatusIcon];
        NSUInteger nextFrame = litDots(delegate.statusItem.button.image);
        if (!firstFrame || !nextFrame || firstFrame == nextFrame) {
            NSLog(@"Loading must change which dots are lit");
            return 1;
        }
        iconState = HTMenuIconReady;
        [delegate refreshStatusIcon];
        NSImage *ready = delegate.statusItem.button.image;
        if (litDots(ready) != 0b000111010) {
            NSLog(@"Ready icon must light the top-center dot and all middle dots");
            return 1;
        }
        CGFloat width = horizontalInkWidth(ready);
        if (width < 15 || width > 17) {
            NSLog(@"The nine dots should occupy about 16 pt horizontally (got %.1f)", width);
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
            if (window.isVisible && containsLabelText(window.contentView, @"Hello TSNet")) { aboutWindow = window; }
        }
        if (!aboutWindow) {
            NSLog(@"About menu did not display the application name");
            return 1;
        }
        if (!containsLink(aboutWindow.contentView, @"https://tailscale.com/docs/features/tsnet")) {
            NSLog(@"About must link to the tsnet documentation");
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
        NSWindow *panel = nil;
        for (NSWindow *window in NSApp.windows) {
            if (window.isVisible && [window.identifier isEqualToString:@"hello-tsnet.devices"]) { panel = window; }
        }
        if (!panel || openedURL != nil) {
            NSLog(@"Control panel must open an AppKit device window without launching a browser");
            return 1;
        }
        NSDictionary *status = @{ @"state": @"ready",
            @"account": @{ @"name": @"Test User", @"login": @"test@example.com", @"tailnet": @"test.ts.net" },
            @"localDevice": @{ @"id": @"self", @"local": @YES, @"name": @"Hello TSNet", @"dnsName": @"hello-tsnet.tailtest.ts.net",
                @"addresses": @[@"100.64.0.1", @"fd7a:115c:a1e0::1"], @"os": @"macOS", @"state": @"online" } };
        NSTableView *table = [delegate.panel valueForKey:@"table"];
        NSTextField *notice = [delegate.panel valueForKey:@"notice"];
        [delegate.panel update:@{ @"status": status, @"deviceError": @"暂时无法读取设备" }];
        if (table.numberOfRows != 1 || notice.hidden || ![notice.stringValue isEqualToString:@"暂时无法读取设备"]) {
            NSLog(@"Unavailable peer status must show a notice and preserve the local row");
            return 1;
        }
        [delegate.panel update:@{
            @"status": status,
            @"devices": @[
                @{ @"id": @"mini", @"name": @"Mac mini", @"dnsName": @"mac-mini.tailtest.ts.net",
                   @"addresses": @[@"100.64.0.2", @"fd7a:115c:a1e0::2"], @"state": @"online" },
                @{ @"name": @"NAS", @"state": @"offline", @"lastSeen": @"2026-11-05T00:11:22.123Z" }
            ]
        }];
        if (!notice.hidden || containsLabelText(panel.contentView, @"设备")) {
            NSLog(@"A ready device page must omit the large title and success notice");
            return 1;
        }
        NSTableColumn *nameColumn = [table tableColumnWithIdentifier:@"name"];
        [panel.contentView layoutSubtreeIfNeeded];
        NSTableRowView *localRow = [table rowViewAtRow:0 makeIfNecessary:YES];
        NSTableRowView *peerRow = [table rowViewAtRow:1 makeIfNecessary:YES];
        if (!containsLabelText(localRow, @"本机") || !localRow.backgroundColor ||
            [localRow.backgroundColor isEqual:peerRow.backgroundColor]) {
            NSLog(@"Local device must have its own row background and badge");
            return 1;
        }
        NSSearchField *search = [delegate.panel valueForKey:@"search"];
        search.stringValue = @"NAS";
        [delegate.panel controlTextDidChange:[NSNotification notificationWithName:NSControlTextDidChangeNotification object:search]];
        if (table.numberOfRows != 2 ||
            ![[delegate.panel tableView:table objectValueForTableColumn:nameColumn row:0] isEqualToString:@"Hello TSNet"] ||
            ![[delegate.panel tableView:table objectValueForTableColumn:nameColumn row:1] isEqualToString:@"NAS"]) {
            NSLog(@"Local device must remain first while searching peers");
            return 1;
        }
        search.stringValue = @"";
        [delegate.panel controlTextDidChange:[NSNotification notificationWithName:NSControlTextDidChangeNotification object:search]];
        // Drive delegate notifications explicitly so the harness does not depend on focus timing.
        NSNotification *focus = [NSNotification notificationWithName:NSWindowDidBecomeKeyNotification object:panel];
        [delegate.panel windowDidBecomeKey:focus];
        NSTableColumn *pingColumn = [table tableColumnWithIdentifier:@"latency"];
        if (!probing || !pingColumn) { NSLog(@"Device panel must start latency probes"); return 1; }
        unsigned long long firstProbeSession = probeSession;
        [delegate.panel updateLatency:@{ @"id": @"mini", @"address": @"100.64.0.2", @"milliseconds": @12.5 } session:probeSession];
        if (![[delegate.panel tableView:table objectValueForTableColumn:pingColumn row:0] isEqualToString:@"—"] ||
            ![[delegate.panel tableView:table objectValueForTableColumn:pingColumn row:1] isEqualToString:@"12.5 ms"] ||
            ![[delegate.panel tableView:table objectValueForTableColumn:pingColumn row:2] isEqualToString:@"—"]) {
            NSLog(@"Online latency and offline placeholders must be distinct"); return 1;
        }
        NSSegmentedControl *tabs = [NSSegmentedControl new];
        tabs.segmentCount = 2;
        tabs.selectedSegment = 1;
        [delegate.panel performSelector:@selector(changePage:) withObject:tabs];
        if (probing) { NSLog(@"Settings must stop latency probes"); return 1; }
        tabs.selectedSegment = 0;
        [delegate.panel performSelector:@selector(changePage:) withObject:tabs];
        [delegate.panel windowDidBecomeKey:focus];
        [delegate.panel updateLatency:@{ @"id": @"mini", @"address": @"100.64.0.2", @"milliseconds": @999 } session:firstProbeSession];
        if (!probing || [[delegate.panel tableView:table objectValueForTableColumn:pingColumn row:1] isEqualToString:@"999.0 ms"]) {
            NSLog(@"Reopening must discard stale latency results"); return 1;
        }
        [delegate.panel windowDidResignKey:[NSNotification notificationWithName:NSWindowDidResignKeyNotification object:panel]];
        if (probing) { NSLog(@"Background panel must stop latency probes"); return 1; }
        [delegate.panel windowDidBecomeKey:focus];
        if (table.numberOfRows != 3 ||
            ![[delegate.panel tableView:table objectValueForTableColumn:nameColumn row:1] isEqualToString:@"Mac mini"]) {
            NSLog(@"Device snapshot did not populate the native table");
            return 1;
        }
        testPasteboard = [NSPasteboard pasteboardWithUniqueName];
        Method general = class_getClassMethod(NSPasteboard.class, @selector(generalPasteboard));
        Method isolated = class_getClassMethod(NSPasteboard.class, @selector(testGeneralPasteboard));
        method_exchangeImplementations(general, isolated);
        [panel setContentSize:panel.contentMinSize];
        [panel.contentView layoutSubtreeIfNeeded];
        NSRect lastColumn = [table rectOfColumn:table.numberOfColumns - 1];
        if (NSMaxX(lastColumn) > NSMaxX(table.visibleRect) + 1) {
            NSLog(@"All columns must remain visible at the minimum window width");
            return 1;
        }
        [panel setContentSize:NSMakeSize(1600, 550)];
        [panel.contentView layoutSubtreeIfNeeded];
        [table sizeToFit];
        [table layoutSubtreeIfNeeded];
        NSButton *wideDomain = findButton(panel.contentView, @"mac-mini.tailtest.ts.net");
        CGFloat domainWidth = ceil(wideDomain.attributedTitle.size.width + wideDomain.image.size.width + 8);
        if (!wideDomain || wideDomain.bounds.size.width + 1 < domainWidth) {
            NSLog(@"Wide name column still truncates domain: button %.1f, needed %.1f, column %.1f",
                wideDomain.bounds.size.width, domainWidth, nameColumn.width);
            return 1;
        }
        [panel setContentSize:panel.contentMinSize];
        [panel.contentView layoutSubtreeIfNeeded];
        for (NSString *address in @[@"100.64.0.1", @"fd7a:115c:a1e0::1", @"100.64.0.2", @"fd7a:115c:a1e0::2",
                                      @"hello-tsnet.tailtest.ts.net", @"mac-mini.tailtest.ts.net", @"mac-mini.local"]) {
            NSButton *button = findButton(panel.contentView, address);
            [button performClick:nil];
            if (!button.image || button.imagePosition != NSImageTrailing ||
                ![[testPasteboard stringForType:NSPasteboardTypeString] isEqualToString:address]) {
                NSLog(@"Each address must be individually copyable: %@", address);
                return 1;
            }
        }
        method_exchangeImplementations(general, isolated);
        [testPasteboard releaseGlobally];
        NSPopover *addressFeedback = [delegate.panel valueForKey:@"addressFeedback"];
        if (!addressFeedback.shown) {
            NSLog(@"Copy feedback must be visible near the address");
            return 1;
        }
        NSTableColumn *numberColumn = [table tableColumnWithIdentifier:@"number"];
        if (!table.usesAlternatingRowBackgroundColors || !numberColumn ||
            ![[delegate.panel tableView:table objectValueForTableColumn:numberColumn row:0] isEqualToString:@"1"] ||
            ![[delegate.panel tableView:table objectValueForTableColumn:numberColumn row:1] isEqualToString:@"2"]) {
            NSLog(@"The table must show row numbers and alternating colors");
            return 1;
        }
        NSInteger stateColumn = [table columnWithIdentifier:@"state"];
        if (stateColumn < 0 || [table tableColumnWithIdentifier:@"lastSeen"]) {
            NSLog(@"Last-seen time must share the status column");
            return 1;
        }
        NSView *online = [table viewAtColumn:stateColumn row:1 makeIfNecessary:YES];
        NSView *offline = [table viewAtColumn:stateColumn row:2 makeIfNecessary:YES];
        NSStackView *onlineLines = (NSStackView *)online.subviews.firstObject;
        NSStackView *offlineLines = (NSStackView *)offline.subviews.firstObject;
        NSView *lastLine = offlineLines.arrangedSubviews.lastObject;
        NSString *seen = [lastLine isKindOfClass:NSTextField.class] ? ((NSTextField *)lastLine).stringValue : @"";
        if (!containsLabelText(online, @"在线") || onlineLines.arrangedSubviews.count != 1 ||
            !containsLabelText(offline, @"离线") || offlineLines.arrangedSubviews.count != 2 ||
            seen.length != 16 || [seen containsString:@"T"]) {
            NSLog(@"Only offline status should include a second line with formatted last-seen time");
            return 1;
        }
        NSTextField *loginLabel = [delegate.panel valueForKey:@"accountLogin"];
        if (![loginLabel.stringValue isEqualToString:@"test@example.com"] || !findButton(panel.contentView, @"退出登录")) {
            NSLog(@"Settings must show the current account and logout action");
            return 1;
        }
        NSButton *uninstall = findButton(panel.contentView, @"卸载路由助手");
        if (!uninstall.enabled || [menu itemWithTitle:@"安装路由助手…"]) {
            NSLog(@"An installed helper must be removable from Settings");
            return 1;
        }
        routeHelperUpgrade = YES;
        [menu.delegate menuNeedsUpdate:menu];
        NSButton *upgrade = findButton(panel.contentView, @"更新路由助手");
        if (!upgrade || upgrade.hidden || !upgrade.enabled) {
            NSLog(@"Settings must offer an update for an older helper");
            return 1;
        }
        routeHelperUpgrade = NO;
        [menu.delegate menuNeedsUpdate:menu];
        Method runModal = class_getInstanceMethod(NSAlert.class, @selector(runModal));
        Method stubModal = class_getInstanceMethod(NSAlert.class, @selector(testRouteHelperResponse));
        method_exchangeImplementations(runModal, stubModal);
        routeHelperResponse = NSAlertSecondButtonReturn;
        [uninstall sendAction:uninstall.action to:uninstall.target];
        BOOL uninstallCancelled = uninstall.enabled && uninstallCount == 0;
        routeHelperResponse = NSAlertFirstButtonReturn;
        [uninstall sendAction:uninstall.action to:uninstall.target];
        method_exchangeImplementations(runModal, stubModal);
        if (!uninstallCancelled || uninstallCount != 1 || uninstall.enabled) {
            NSLog(@"Removing the helper must require consent and disable controls until completion");
            return 1;
        }
        routeHelperNeeded = YES;
        [delegate routeHelperFinished:@"" removing:YES];
        if (![menu itemWithTitle:@"安装路由助手…"] || !findButton(panel.contentView, @"安装路由助手").enabled) {
            NSLog(@"Successful removal must restore installation in the menu and Settings");
            return 1;
        }
        routeHelperNeeded = NO;
        [menu.delegate menuNeedsUpdate:menu];
        if (quitCount != 0) { return 1; }
        signedIn = 0;
        [menu.delegate menuNeedsUpdate:menu];
        [menu performActionForItemAtIndex:1];
        if ([menu itemWithTitle:@"控制面板"] || ![menu itemWithTitle:@"授权登录"] || panel.isVisible || authorizeCount != 1 || probing) {
            NSLog(@"Signed-out menu must replace the dashboard with authorization");
            return 1;
        }
        signedIn = 1;
        [menu.delegate menuNeedsUpdate:menu];
        routeHelperNeeded = YES;
        HTAppDelegate *setupDelegate = [HTAppDelegate new];
        [setupDelegate applicationDidFinishLaunching:[NSNotification notificationWithName:NSApplicationDidFinishLaunchingNotification object:NSApp]];
        NSMenuItem *install = [setupDelegate.statusItem.menu itemWithTitle:@"安装路由助手…"];
        if (!install || installCount != 0) {
            NSLog(@"A missing route helper must offer installation without running it before consent");
            return 1;
        }
        method_exchangeImplementations(runModal, stubModal);
        routeHelperResponse = NSAlertSecondButtonReturn;
        [setupDelegate installRouteHelper:nil];
        BOOL cancelled = install.enabled && installCount == 0;
        routeHelperResponse = NSAlertFirstButtonReturn;
        [setupDelegate installRouteHelper:nil];
        [setupDelegate installRouteHelper:nil];
        method_exchangeImplementations(runModal, stubModal);
        if (!cancelled || installCount != 1 || install.enabled) {
            NSLog(@"Installation must require consent and run only once at a time");
            return 1;
        }
        [NSStatusBar.systemStatusBar removeStatusItem:setupDelegate.statusItem];
        [menu performActionForItemAtIndex:2];
        [delegate quit:nil];
        if (quitCount != 1 || menu.itemArray.lastObject.enabled) {
            NSLog(@"Quit must cancel once and disable the menu");
            return 1;
        }
        [NSStatusBar.systemStatusBar removeStatusItem:delegate.statusItem];
        puts("Native menu: action wiring, animated and ready icons, About open/close, native device panel, idempotent quit passed");
    }
    return 0;
}
