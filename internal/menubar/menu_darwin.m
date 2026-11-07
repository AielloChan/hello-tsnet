#import <AppKit/AppKit.h>
#import "menu.h"
#import "panel.h"

extern void goMenuQuit(void);
extern void goMenuRefresh(void);
extern int goMenuSignedIn(void);
extern int goMenuIconState(void);
extern void goMenuAuthorize(void);
extern void goMenuLogout(void);
extern int goMenuRouteHelperNeeded(void);
extern int goMenuRouteHelperNeedsUpgrade(void);
extern void goMenuInstallRouteHelper(void);
extern void goMenuUninstallRouteHelper(void);

@interface HTAppDelegate : NSObject <NSApplicationDelegate, NSMenuDelegate>
@property(nonatomic, strong) NSStatusItem *statusItem;
@property(nonatomic, strong) HTDevicePanel *panel;
@property(nonatomic, strong) NSMenuItem *panelItem;
@property(nonatomic, strong) NSMenuItem *routeHelperItem;
@property(nonatomic, strong) NSTimer *iconTimer;
@property(nonatomic, strong) NSAppearance *iconAppearance;
@property(nonatomic) NSUInteger litDots;
@property(nonatomic) BOOL routeHelperBusy;
@property(nonatomic) BOOL quitting;
@property(nonatomic) BOOL systemTermination;
@end

static NSImage *menuIcon(NSUInteger lit, NSAppearance *appearance) {
    return [NSImage imageWithSize:NSMakeSize(20, 20) flipped:NO drawingHandler:^BOOL(NSRect rect) {
        [appearance performAsCurrentDrawingAppearance:^{
            NSColor *pink = HTThemeAccentColor();
            NSString *mode = [appearance bestMatchFromAppearancesWithNames:@[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]];
            NSColor *unlit = [mode isEqualToString:NSAppearanceNameDarkAqua] ? NSColor.whiteColor : NSColor.blackColor;
            for (NSUInteger row = 0; row < 3; row++) {
                for (NSUInteger column = 0; column < 3; column++) {
                    BOOL isLit = (lit & (1u << (row * 3 + column))) != 0;
                    [(isLit ? pink : unlit) setFill];
                    NSRect dot = NSMakeRect(rect.origin.x + 2 + column * 6,
                                           rect.origin.y + 2 + (2 - row) * 6, 4, 4);
                    [[NSBezierPath bezierPathWithOvalInRect:dot] fill];
                }
            }
        }];
        return YES;
    }];
}

static void showError(NSString *message) {
    [NSApp activateIgnoringOtherApps:YES];
    NSAlert *alert = [NSAlert new];
    alert.messageText = @"Hello TSNet 无法继续运行";
    alert.informativeText = message;
    alert.alertStyle = NSAlertStyleWarning;
    [alert addButtonWithTitle:@"好"];
    [alert runModal];
}

@implementation HTAppDelegate
- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    self.statusItem = [NSStatusBar.systemStatusBar statusItemWithLength:NSSquareStatusItemLength];
    [self.statusItem.button setAccessibilityLabel:@"Hello TSNet"];
    [self refreshStatusIcon];
    NSMenu *menu = [[NSMenu alloc] initWithTitle:@"Hello TSNet"];
    menu.autoenablesItems = NO;
    menu.delegate = self;
    NSArray *titles = @[@"关于", @"控制面板", @"退出应用"];
    SEL actions[] = {@selector(about:), @selector(controlPanel:), @selector(quit:)};
    for (NSUInteger index = 0; index < titles.count; index++) {
        NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:titles[index] action:actions[index] keyEquivalent:@""];
        item.target = self;
        [menu addItem:item];
    }
    self.statusItem.menu = menu;
    self.panelItem = [menu itemAtIndex:1];
    [self menuNeedsUpdate:menu];
    if (self.routeHelperItem) {
        dispatch_async(dispatch_get_main_queue(), ^{ [self installRouteHelper:nil]; });
    }
}

- (void)refreshStatusIcon {
    if (self.quitting) { return; }
    int state = goMenuIconState();
    BOOL loading = state == HTMenuIconLoading;
    // Top center and right, middle left and center, bottom right.
    const NSUInteger ready = 0b100011110;
    NSUInteger lit = state == HTMenuIconReady ? ready : 0;
    if (loading) {
        do {
            lit = arc4random_uniform(510) + 1;
        } while (lit == self.litDots || lit == ready);
    }
    NSStatusBarButton *button = self.statusItem.button;
    NSAppearance *appearance = button.effectiveAppearance;
    if (!button.image || lit != self.litDots || ![appearance isEqual:self.iconAppearance]) {
        self.litDots = lit;
        self.iconAppearance = appearance;
        button.image = menuIcon(lit, appearance);
    }
    button.toolTip = loading ? @"Hello TSNet 正在连接或配置路由…" : @"Hello TSNet";
    NSTimeInterval interval = loading ? 0.28 : 1.0;
    if (!self.iconTimer || self.iconTimer.timeInterval != interval) {
        [self.iconTimer invalidate];
        __weak HTAppDelegate *weakSelf = self;
        self.iconTimer = [NSTimer timerWithTimeInterval:interval repeats:YES block:^(NSTimer *timer) {
            if (weakSelf) { [weakSelf refreshStatusIcon]; }
            else { [timer invalidate]; }
        }];
        self.iconTimer.tolerance = interval * 0.2;
        [NSRunLoop.mainRunLoop addTimer:self.iconTimer forMode:NSRunLoopCommonModes];
    }
}

- (void)updateRouteHelperControls {
    BOOL needsInstall = goMenuRouteHelperNeeded() != 0;
    NSMenu *menu = self.statusItem.menu;
    if (needsInstall && !self.routeHelperItem) {
        self.routeHelperItem = [[NSMenuItem alloc] initWithTitle:@"安装路由助手…"
            action:@selector(installRouteHelper:) keyEquivalent:@""];
        self.routeHelperItem.target = self;
        [menu insertItem:self.routeHelperItem atIndex:menu.numberOfItems - 1];
    } else if (!needsInstall && self.routeHelperItem) {
        [menu removeItem:self.routeHelperItem];
        self.routeHelperItem = nil;
    }
    self.routeHelperItem.enabled = !self.routeHelperBusy;
    [self.panel updateRouteHelperInstalled:!needsInstall
        upgrade:!needsInstall && goMenuRouteHelperNeedsUpgrade() busy:self.routeHelperBusy];
}

- (void)installRouteHelper:(id)sender { [self requestRouteHelperAction:HTRouteHelperInstall]; }

- (void)requestRouteHelperAction:(HTRouteHelperAction)action {
    if (self.quitting || self.routeHelperBusy) { return; }
    BOOL needsInstall = goMenuRouteHelperNeeded() != 0;
    BOOL removing = action == HTRouteHelperRemove;
    BOOL upgrading = action == HTRouteHelperUpgrade;
    if ((action == HTRouteHelperInstall) != needsInstall || (upgrading && !goMenuRouteHelperNeedsUpgrade())) {
        [self updateRouteHelperControls];
        return;
    }
    self.routeHelperBusy = YES;
    [self updateRouteHelperControls];
    [NSApp activateIgnoringOtherApps:YES];
    NSAlert *alert = [NSAlert new];
    alert.messageText = removing ? @"卸载 Hello TSNet 路由助手"
        : (upgrading ? @"更新 Hello TSNet 路由助手" : @"安装 Hello TSNet 路由助手");
    alert.informativeText = removing
        ? @"卸载会断开通过 Hello TSNet 直连 Tailnet 设备的连接。macOS 会要求管理员授权。"
        : @"助手会配置 Tailnet 设备路由，并通过 hosts 为本机提供 .ts.net 和 .local 名称解析。macOS 会要求管理员授权。";
    [alert addButtonWithTitle:removing ? @"卸载" : (upgrading ? @"更新" : @"安装")].hasDestructiveAction = removing;
    [alert addButtonWithTitle:removing ? @"取消" : @"稍后"].keyEquivalent = @"\033";
    if ([alert runModal] == NSAlertFirstButtonReturn && !self.quitting) {
        if (removing) { goMenuUninstallRouteHelper(); }
        else { goMenuInstallRouteHelper(); }
    } else if (!self.quitting) {
        self.routeHelperBusy = NO;
        [self updateRouteHelperControls];
    }
}

- (void)routeHelperFinished:(NSString *)message removing:(BOOL)removing {
    if (self.quitting) { return; }
    self.routeHelperBusy = NO;
    [self updateRouteHelperControls];
    [self refreshStatusIcon];
    goMenuRefresh();
    if (message.length > 0) {
        NSAlert *alert = [NSAlert new];
        alert.messageText = removing ? @"路由助手卸载失败" : @"路由助手安装失败";
        alert.informativeText = message;
        [alert addButtonWithTitle:@"好"];
        [NSApp activateIgnoringOtherApps:YES];
        [alert runModal];
    }
}

- (void)about:(id)sender {
    [NSApp activateIgnoringOtherApps:YES];
    NSString *version = [NSBundle.mainBundle objectForInfoDictionaryKey:@"CFBundleShortVersionString"] ?: @"开发版";
    NSMutableParagraphStyle *paragraph = [NSMutableParagraphStyle new];
    paragraph.alignment = NSTextAlignmentCenter;
    paragraph.lineSpacing = 4;
    paragraph.paragraphSpacing = 10;
    NSMutableAttributedString *credits = [[NSMutableAttributedString alloc]
        initWithString:@"轻量的 Tailscale 设备连接工具\n"
        attributes:@{
            NSFontAttributeName: [NSFont systemFontOfSize:14 weight:NSFontWeightMedium],
            NSForegroundColorAttributeName: NSColor.labelColor,
            NSParagraphStyleAttributeName: paragraph
        }];
    [credits appendAttributedString:[[NSAttributedString alloc] initWithString:@"Tailscale tsnet 文档" attributes:@{
        NSFontAttributeName: [NSFont systemFontOfSize:12],
        NSForegroundColorAttributeName: NSColor.linkColor,
        NSLinkAttributeName: [NSURL URLWithString:@"https://tailscale.com/docs/features/tsnet"],
        NSParagraphStyleAttributeName: paragraph
    }]];
    [NSApp orderFrontStandardAboutPanelWithOptions:@{
        NSAboutPanelOptionApplicationName: @"Hello TSNet",
        NSAboutPanelOptionApplicationVersion: version,
        NSAboutPanelOptionVersion: @"",
        NSAboutPanelOptionCredits: credits
    }];
}

- (void)controlPanel:(id)sender {
    if (!goMenuSignedIn()) {
        [self menuNeedsUpdate:self.statusItem.menu];
        return;
    }
    if (!self.panel) {
        self.panel = [HTDevicePanel new];
        __weak HTAppDelegate *weakSelf = self;
        self.panel.routeHelperAction = ^(HTRouteHelperAction action) {
            [weakSelf requestRouteHelperAction:action];
        };
    }
    [self updateRouteHelperControls];
    [self.panel show];
}

- (void)authorizeLogin:(id)sender { goMenuAuthorize(); }

- (void)menuNeedsUpdate:(NSMenu *)menu {
    if (self.quitting) { return; }
    [self updateRouteHelperControls];
    BOOL signedIn = goMenuSignedIn() != 0;
    self.panelItem.title = signedIn ? @"控制面板" : @"授权登录";
    self.panelItem.action = signedIn ? @selector(controlPanel:) : @selector(authorizeLogin:);
    if (!signedIn && self.panel) {
        [self.panel close];
        self.panel = nil;
    }
}

- (void)accountFinished:(NSString *)url error:(NSString *)error logoutResult:(int)logoutResult {
    if (self.quitting) { return; }
    if (logoutResult > 0) {
        [self.panel close];
        self.panel = nil;
    }
    [self menuNeedsUpdate:self.statusItem.menu];
    [self.panel resetAccountAction];
    if (error.length) {
        NSAlert *alert = [NSAlert new];
        alert.messageText = @"Tailscale 账号";
        alert.informativeText = error;
        BOOL retryLogout = logoutResult < 0;
        [alert addButtonWithTitle:retryLogout ? @"重试退出" : @"好"];
        if (retryLogout) { [alert addButtonWithTitle:@"取消"].keyEquivalent = @"\033"; }
        void (^complete)(NSModalResponse) = ^(NSModalResponse response) {
            if (retryLogout && response == NSAlertFirstButtonReturn && !self.quitting) {
                goMenuLogout();
            }
        };
        if (self.panel.window.isVisible) {
            [alert beginSheetModalForWindow:self.panel.window completionHandler:complete];
        } else {
            [NSApp activateIgnoringOtherApps:YES];
            complete([alert runModal]);
        }
    } else if (url.length) {
        [NSWorkspace.sharedWorkspace openURL:[NSURL URLWithString:url]];
    }
}

- (void)quit:(id)sender {
    if (self.quitting) { return; }
    self.quitting = YES;
    [self.iconTimer invalidate];
    for (NSMenuItem *item in self.statusItem.menu.itemArray) { item.enabled = NO; }
    self.statusItem.button.toolTip = @"Hello TSNet 正在退出…";
    goMenuQuit();
}

- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
    self.systemTermination = YES;
    [self quit:nil];
    return NSTerminateLater;
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender { return NO; }
@end

void HTMenuRun(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        HTAppDelegate *delegate = [HTAppDelegate new];
        NSApp.delegate = delegate;
        [NSApp run];
        [NSStatusBar.systemStatusBar removeStatusItem:delegate.statusItem];
        NSApp.delegate = nil;
    }
}

void HTMenuUpdate(const char *json) {
    @autoreleasepool {
        NSString *value = [NSString stringWithUTF8String:json];
        NSData *data = [value dataUsingEncoding:NSUTF8StringEncoding];
        NSDictionary *snapshot = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        if (!snapshot) { return; }
        dispatch_async(dispatch_get_main_queue(), ^{
            HTAppDelegate *delegate = (HTAppDelegate *)NSApp.delegate;
            [delegate menuNeedsUpdate:delegate.statusItem.menu];
            [delegate.panel update:snapshot];
        });
    }
}

void HTMenuLatencyUpdate(const char *json, unsigned long long session) {
    @autoreleasepool {
        NSData *data = [[NSString stringWithUTF8String:json] dataUsingEncoding:NSUTF8StringEncoding];
        NSDictionary *result = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        if (!result) { return; }
        dispatch_async(dispatch_get_main_queue(), ^{
            HTAppDelegate *delegate = (HTAppDelegate *)NSApp.delegate;
            [delegate.panel updateLatency:result session:session];
        });
    }
}

void HTMenuAccountFinished(const char *url, const char *errorMessage, int logoutResult) {
    @autoreleasepool {
        NSString *link = [NSString stringWithUTF8String:url];
        NSString *error = [NSString stringWithUTF8String:errorMessage];
        dispatch_async(dispatch_get_main_queue(), ^{
            HTAppDelegate *delegate = (HTAppDelegate *)NSApp.delegate;
            [delegate accountFinished:link error:error logoutResult:logoutResult];
        });
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

void HTMenuRouteHelperFinished(const char *message, int removing) {
    @autoreleasepool {
        NSString *text = [NSString stringWithUTF8String:message];
        dispatch_async(dispatch_get_main_queue(), ^{
            HTAppDelegate *delegate = (HTAppDelegate *)NSApp.delegate;
            [delegate routeHelperFinished:text removing:removing != 0];
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
