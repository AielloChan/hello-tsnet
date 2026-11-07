#import <AppKit/AppKit.h>
#import <ServiceManagement/ServiceManagement.h>
#import <objc/runtime.h>
#import "../internal/menubar/avatar.h"
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

@interface HTTestLoginItem : NSObject
@property(nonatomic) SMAppServiceStatus status;
@property(nonatomic) NSUInteger registrations;
@property(nonatomic) NSUInteger removals;
@end
@implementation HTTestLoginItem
- (BOOL)registerAndReturnError:(NSError **)error {
    self.registrations++;
    self.status = SMAppServiceStatusRequiresApproval;
    return YES;
}
- (BOOL)unregisterAndReturnError:(NSError **)error {
    self.removals++;
    self.status = SMAppServiceStatusNotRegistered;
    return YES;
}
@end

static HTTestLoginItem *testLoginItem;
@interface SMAppService (MenuTests)
+ (SMAppService *)testMainAppService;
@end
@implementation SMAppService (MenuTests)
+ (SMAppService *)testMainAppService { return (SMAppService *)testLoginItem; }
@end

@interface HTTestAvatarProtocol : NSURLProtocol
@end
@implementation HTTestAvatarProtocol
+ (BOOL)canInitWithRequest:(NSURLRequest *)request {
    return [request.URL.host isEqualToString:@"avatars.example.invalid"];
}
+ (NSURLRequest *)canonicalRequestForRequest:(NSURLRequest *)request { return request; }
- (void)startLoading {
    NSData *png = [[NSData alloc] initWithBase64EncodedString:
        @"iVBORw0KGgoAAAANSUhEUgAAAAQAAAACCAIAAADwyuo0AAAAEElEQVR4nGPgrzgBRwzIHACCagp5d+QGSAAAAABJRU5ErkJggg==" options:0];
    NSHTTPURLResponse *response = [[NSHTTPURLResponse alloc] initWithURL:self.request.URL statusCode:200
        HTTPVersion:@"HTTP/1.1" headerFields:@{@"Content-Type": @"image/png"}];
    [self.client URLProtocol:self didReceiveResponse:response cacheStoragePolicy:NSURLCacheStorageNotAllowed];
    [self.client URLProtocol:self didLoadData:png];
    [self.client URLProtocolDidFinishLoading:self];
}
- (void)stopLoading {}
@end

@interface HTAccountAvatarView (MenuTests)
+ (NSURLSessionConfiguration *)testPhotoSessionConfiguration;
@end
@implementation HTAccountAvatarView (MenuTests)
+ (NSURLSessionConfiguration *)testPhotoSessionConfiguration {
    NSURLSessionConfiguration *configuration = [self testPhotoSessionConfiguration];
    configuration.protocolClasses = @[HTTestAvatarProtocol.class];
    return configuration;
}
@end

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

static NSTextField *findLabelText(NSView *view, NSString *text) {
    if ([view isKindOfClass:NSTextField.class] &&
        [((NSTextField *)view).stringValue isEqualToString:text]) { return (NSTextField *)view; }
    for (NSView *child in view.subviews) {
        NSTextField *field = findLabelText(child, text);
        if (field) { return field; }
    }
    return nil;
}

static BOOL containsLabelText(NSView *view, NSString *text) { return findLabelText(view, text) != nil; }

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
        NSString *preferencesSuite = [@"org.hellotsnet.native-test." stringByAppendingString:NSUUID.UUID.UUIDString];
        NSUserDefaults *preferences = [[NSUserDefaults alloc] initWithSuiteName:preferencesSuite];
        testLoginItem = [HTTestLoginItem new];
        testLoginItem.status = SMAppServiceStatusNotFound;
        Method mainAppService = class_getClassMethod(SMAppService.class, @selector(mainAppService));
        Method stubMainAppService = class_getClassMethod(SMAppService.class, @selector(testMainAppService));
        method_exchangeImplementations(mainAppService, stubMainAppService);
        HTAppDelegate *delegate = [HTAppDelegate new];
        delegate.preferences = preferences;
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
        NSImage *lightIcon = menuIcon(0, [NSAppearance appearanceNamed:NSAppearanceNameAqua], nil);
        NSImage *darkIcon = menuIcon(0, [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua], nil);
        NSColor *lightDot = dotColor([NSBitmapImageRep imageRepWithData:lightIcon.TIFFRepresentation], 0, 0);
        NSColor *darkDot = dotColor([NSBitmapImageRep imageRepWithData:darkIcon.TIFFRepresentation], 0, 0);
        if (lightDot.alphaComponent < 0.9 || lightDot.redComponent > 0.15 ||
            darkDot.alphaComponent < 0.9 || darkDot.greenComponent < 0.85) {
            NSLog(@"Unlit dots must be black in light mode and white in dark mode");
            return 1;
        }
        for (NSString *mode in @[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]) {
            CGFloat neutral = [mode isEqualToString:NSAppearanceNameDarkAqua] ? 1 : 0;
            NSColor *custom = [NSColor colorWithSRGBRed:neutral green:neutral blue:neutral alpha:1];
            NSImage *image = menuIcon(1u << 1, [NSAppearance appearanceNamed:mode], custom);
            NSBitmapImageRep *bitmap = [NSBitmapImageRep imageRepWithData:image.TIFFRepresentation];
            NSColor *lit = dotColor(bitmap, 0, 1);
            NSColor *unlit = dotColor(bitmap, 0, 0);
            if (lit.alphaComponent - unlit.alphaComponent < 0.25) {
                NSLog(@"A custom highlight matching the unlit color must still reveal the dot pattern");
                return 1;
            }
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
        if (litDots(ready) != 0b100011110) {
            NSLog(@"Ready icon must light the top two right dots, middle two left dots, and bottom-right dot");
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
        NSView *settings = [delegate.panel valueForKey:@"settingsPage"];
        if (containsLabelText(settings, @"设置") || containsLabelText(settings, @"Tailscale 账号") ||
            !containsLabelText(settings, @"Test User") || !containsLabelText(settings, @"test@example.com") ||
            !findButton(settings, @"退出登录")) {
            NSLog(@"Settings must start with the signed-in identity and logout without a card title");
            return 1;
        }
        HTAccountAvatarView *avatar = [delegate.panel valueForKey:@"accountAvatar"];
        NSImage *placeholder = avatar.image;
        [panel.contentView layoutSubtreeIfNeeded];
        if (!placeholder || NSWidth(avatar.bounds) != 44 || NSHeight(avatar.bounds) != 44) {
            NSLog(@"An account without a photo must show a 44 pt placeholder avatar (image %d, frame %@, alignment %@)",
                placeholder != nil, NSStringFromRect(avatar.frame), NSStringFromRect([avatar alignmentRectForFrame:avatar.frame]));
            return 1;
        }
        Method photoConfiguration = class_getClassMethod(HTAccountAvatarView.class, @selector(photoSessionConfiguration));
        Method stubPhotoConfiguration = class_getClassMethod(HTAccountAvatarView.class, @selector(testPhotoSessionConfiguration));
        method_exchangeImplementations(photoConfiguration, stubPhotoConfiguration);
        NSMutableDictionary *photoStatus = [status mutableCopy];
        NSMutableDictionary *photoAccount = [status[@"account"] mutableCopy];
        photoStatus[@"account"] = photoAccount;
        photoAccount[@"profilePicURL"] = @"https://avatars.example.invalid/account.png";
        [delegate.panel update:@{@"status": photoStatus}];
        NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:2];
        while (avatar.image == placeholder && deadline.timeIntervalSinceNow > 0) {
            [NSRunLoop.currentRunLoop runMode:NSDefaultRunLoopMode beforeDate:deadline];
        }
        if (!avatar.image || avatar.image == placeholder) {
            NSLog(@"The current account photo must replace its placeholder");
            return 1;
        }
        if (avatar.image.size.width != avatar.image.size.height) {
            NSLog(@"Non-square account photos must fill the circular avatar frame");
            return 1;
        }
        photoAccount[@"login"] = @"other@example.com";
        [delegate.panel update:@{@"status": photoStatus}];
        if (avatar.image != placeholder || ![avatar valueForKey:@"photoTask"]) {
            NSLog(@"Changing accounts must restart loading even if the photo URL is the same");
            return 1;
        }
        photoAccount[@"profilePicURL"] = @"http://avatars.example.invalid/account.png";
        [delegate.panel update:@{@"status": photoStatus}];
        method_exchangeImplementations(photoConfiguration, stubPhotoConfiguration);
        if (avatar.image != placeholder || [avatar valueForKey:@"photoTask"]) {
            NSLog(@"An insecure photo URL must clear the avatar and keep the placeholder");
            return 1;
        }
        [delegate.panel update:@{@"status": status}];
        if (!containsLabelText(settings, @"菜单栏亮点颜色") || !containsLabelText(settings, @"登录时启动") ||
            !findButton(settings, @"恢复默认颜色")) {
            NSLog(@"Settings must expose menu bar color and launch at login");
            return 1;
        }
        HTPreferencesView *options = delegate.panel.preferencesView;
        NSSize previousSize = panel.contentView.bounds.size;
        [panel setContentSize:panel.contentMinSize];
        [panel.contentView layoutSubtreeIfNeeded];
        NSRect optionsFrame = [settings convertRect:options.bounds fromView:options];
        if (NSIsEmptyRect(optionsFrame) || !NSContainsRect(settings.bounds, optionsFrame)) {
            NSLog(@"Both preferences must fit at the minimum window size");
            return 1;
        }
        NSTextField *helperDescription = findLabelText(settings,
            @"助手为 Tailscale 设备设置专用路由，通过 macOS 虚拟网卡（utun）把流量交给 Hello TSNet，让本机应用直接访问这些设备。新版助手还会同步本机的名称解析（hosts），让你用设备域名直接连接。");
        NSRect descriptionFrame = [settings convertRect:helperDescription.bounds fromView:helperDescription];
        if (!helperDescription || NSIsEmptyRect(descriptionFrame) || !NSContainsRect(settings.bounds, descriptionFrame) ||
            NSIntersectsRect(descriptionFrame, optionsFrame)) {
            NSLog(@"The route helper explanation must fit above the preferences at minimum size");
            return 1;
        }
        __block CGFloat lightText = 0, darkText = 0;
        __block BOOL lightIsSubtle = NO, darkIsSubtle = NO;
        NSTextField *helperStatus = [delegate.panel valueForKey:@"routeHelperStatus"];
        [[NSAppearance appearanceNamed:NSAppearanceNameAqua] performAsCurrentDrawingAppearance:^{
            NSColor *description = [helperDescription.textColor colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
            lightText = description.redComponent;
            lightIsSubtle = description.alphaComponent < [helperStatus.textColor colorUsingColorSpace:NSColorSpace.sRGBColorSpace].alphaComponent;
        }];
        [[NSAppearance appearanceNamed:NSAppearanceNameDarkAqua] performAsCurrentDrawingAppearance:^{
            NSColor *description = [helperDescription.textColor colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
            darkText = description.redComponent;
            darkIsSubtle = description.alphaComponent < [helperStatus.textColor colorUsingColorSpace:NSColorSpace.sRGBColorSpace].alphaComponent;
        }];
        if (lightText >= darkText) { NSLog(@"Helper description must adapt to light and dark appearance"); return 1; }
        if (!lightIsSubtle || !darkIsSubtle) {
            NSLog(@"Helper description must be more subdued than its installation status in both appearances");
            return 1;
        }
        [panel setContentSize:previousSize];
        [panel.contentView layoutSubtreeIfNeeded];
        NSColorWell *colorWell = [options valueForKey:@"menuBarColorWell"];
        NSButton *resetColor = findButton(settings, @"恢复默认颜色");
        NSColor *customColor = [NSColor colorWithSRGBRed:0.05 green:0.4 blue:0.85 alpha:1];
        colorWell.color = customColor;
        [colorWell sendAction:colorWell.action to:colorWell.target];
        NSColor *changedDot = dotColor([NSBitmapImageRep imageRepWithData:delegate.statusItem.button.image.TIFFRepresentation], 0, 1);
        if (!resetColor.enabled || changedDot.blueComponent < 0.75 || changedDot.redComponent > 0.15 || !storedMenuBarColor(preferences)) {
            NSLog(@"Custom color must redraw the ready icon immediately and persist");
            return 1;
        }
        [resetColor performClick:nil];
        if (resetColor.enabled || storedMenuBarColor(preferences) || litDots(delegate.statusItem.button.image) != 0b100011110) {
            NSLog(@"Reset must restore the default ready icon and remove the override");
            return 1;
        }
        colorWell.color = customColor;
        [colorWell sendAction:colorWell.action to:colorWell.target];

        NSSwitch *login = [options valueForKey:@"loginItemSwitch"];
        NSButton *openLoginItems = findButton(settings, @"打开登录项设置");
        if (login.state != NSControlStateValueOff || !login.enabled || !openLoginItems.hidden || testLoginItem.registrations) {
            NSLog(@"A missing login item must allow first registration without registering automatically");
            return 1;
        }
        login.state = NSControlStateValueOn;
        [login sendAction:login.action to:login.target];
        if (testLoginItem.registrations != 1 || openLoginItems.hidden || !containsLabelText(settings, @"需在系统设置中允许")) {
            NSLog(@"A login item awaiting approval must offer System Settings");
            return 1;
        }
        testLoginItem.status = SMAppServiceStatusEnabled;
        [delegate.panel windowDidBecomeKey:focus];
        if (login.state != NSControlStateValueOn || !openLoginItems.hidden || !containsLabelText(settings, @"已开启")) {
            NSLog(@"Returning from System Settings must refresh login item approval");
            return 1;
        }
        login.state = NSControlStateValueOff;
        [login sendAction:login.action to:login.target];
        if (testLoginItem.removals != 1 || !containsLabelText(settings, @"已关闭")) {
            NSLog(@"Turning off launch at login must remove the system registration");
            return 1;
        }
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
        setupDelegate.preferences = preferences;
        [setupDelegate applicationDidFinishLaunching:[NSNotification notificationWithName:NSApplicationDidFinishLaunchingNotification object:NSApp]];
        NSColor *restoredDot = dotColor([NSBitmapImageRep imageRepWithData:setupDelegate.statusItem.button.image.TIFFRepresentation], 0, 1);
        if (restoredDot.blueComponent < 0.75 || restoredDot.redComponent > 0.15) {
            NSLog(@"Restarting the menu must restore the saved color");
            return 1;
        }
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
        method_exchangeImplementations(mainAppService, stubMainAppService);
        [preferences removePersistentDomainForName:preferencesSuite];
        puts("Native menu: action wiring, animated and ready icons, About open/close, native device panel, idempotent quit passed");
    }
    return 0;
}
