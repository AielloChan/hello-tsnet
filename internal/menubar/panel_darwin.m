#import "panel.h"
#import "avatar.h"
#import "preferences.h"

extern void goMenuRefresh(void);
extern void goMenuLogout(void);
extern unsigned long long goMenuSetProbing(int active);

NSColor *HTThemeAccentColor(void) {
    return [NSColor colorWithName:nil dynamicProvider:^NSColor *(NSAppearance *appearance) {
        BOOL dark = [[appearance bestMatchFromAppearancesWithNames:@[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]]
            isEqualToString:NSAppearanceNameDarkAqua];
        return dark ? [NSColor colorWithSRGBRed:1 green:0.63 blue:0.79 alpha:1]
                    : [NSColor colorWithSRGBRed:0.62 green:0.13 blue:0.34 alpha:1];
    }];
}

static NSTextField *label(NSString *title, CGFloat size, NSFontWeight weight, NSColor *color) {
    NSTextField *field = [NSTextField labelWithString:title];
    field.font = [NSFont systemFontOfSize:size weight:weight];
    field.textColor = color;
    field.lineBreakMode = NSLineBreakByTruncatingTail;
    [field setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow
                                    forOrientation:NSLayoutConstraintOrientationHorizontal];
    return field;
}

static NSString *lastSeenTime(NSString *timestamp) {
    if (!timestamp.length) { return @"—"; }
    static NSISO8601DateFormatter *seconds, *fractionalSeconds;
    static NSDateFormatter *display;
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        seconds = [NSISO8601DateFormatter new];
        fractionalSeconds = [NSISO8601DateFormatter new];
        fractionalSeconds.formatOptions = NSISO8601DateFormatWithInternetDateTime | NSISO8601DateFormatWithFractionalSeconds;
        display = [NSDateFormatter new];
        display.locale = [NSLocale localeWithLocaleIdentifier:@"en_US_POSIX"];
        display.dateFormat = @"yyyy-MM-dd HH:mm";
    });
    NSDate *date = [seconds dateFromString:timestamp] ?: [fractionalSeconds dateFromString:timestamp];
    display.timeZone = NSTimeZone.localTimeZone;
    return date ? [display stringFromDate:date] : @"—";
}

static NSStackView *column(NSArray<NSView *> *views, CGFloat spacing) {
    NSStackView *stack = [NSStackView stackViewWithViews:views];
    stack.orientation = NSUserInterfaceLayoutOrientationVertical;
    stack.alignment = NSLayoutAttributeLeading;
    stack.spacing = spacing;
    return stack;
}

static NSBox *card(NSView *view, NSSize padding) {
    NSBox *box = [NSBox new];
    box.boxType = NSBoxCustom;
    box.titlePosition = NSNoTitle;
    box.cornerRadius = 10;
    box.borderWidth = 1;
    box.borderColor = NSColor.separatorColor;
    box.fillColor = NSColor.controlBackgroundColor;
    box.contentViewMargins = NSZeroSize;
    NSView *content = box.contentView;
    view.translatesAutoresizingMaskIntoConstraints = NO;
    [content addSubview:view];
    [NSLayoutConstraint activateConstraints:@[
        [view.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:padding.width],
        [view.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-padding.width],
        [view.topAnchor constraintEqualToAnchor:content.topAnchor constant:padding.height],
        [view.bottomAnchor constraintEqualToAnchor:content.bottomAnchor constant:-padding.height]
    ]];
    return box;
}

@interface HTDevicePanel ()
@property(nonatomic, strong) NSView *devicePage;
@property(nonatomic, strong) NSView *settingsPage;
@property(nonatomic, strong, readwrite) HTPreferencesView *preferencesView;
@property(nonatomic, strong) NSTextField *tailnet;
@property(nonatomic, strong) NSTextField *notice;
@property(nonatomic, strong) NSTextField *deviceCount;
@property(nonatomic, strong) NSPopover *addressFeedback;
@property(nonatomic, strong) NSTextField *accountName;
@property(nonatomic, strong) NSTextField *accountLogin;
@property(nonatomic, strong) HTAccountAvatarView *accountAvatar;
@property(nonatomic, copy) NSDictionary *avatarAccount;
@property(nonatomic, strong) NSButton *logout;
@property(nonatomic, strong) NSTextField *routeHelperStatus;
@property(nonatomic, strong) NSButton *routeHelperButton;
@property(nonatomic, strong) NSButton *routeHelperUpgradeButton;
@property(nonatomic, strong) NSSearchField *search;
@property(nonatomic, strong) NSTableView *table;
@property(nonatomic, copy) NSDictionary *localDevice;
@property(nonatomic, copy) NSArray<NSDictionary *> *devices;
@property(nonatomic, copy) NSArray<NSDictionary *> *visibleDevices;
@property(nonatomic, strong) NSTimer *timer;
@property(nonatomic, strong) NSTimer *feedbackTimer;
@property(nonatomic) unsigned long long probeSession;
@property(nonatomic, strong) NSMutableDictionary<NSString *, NSDictionary *> *latencies;
@end

@implementation HTDevicePanel

- (instancetype)init {
    NSWindow *window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1040, 550)
        styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskResizable
        backing:NSBackingStoreBuffered defer:NO];
    self = [super initWithWindow:window];
    if (!self) { return nil; }

    window.identifier = @"hello-tsnet.devices";
    window.title = @"Hello TSNet";
    window.contentMinSize = NSMakeSize(930, 410);
    window.releasedWhenClosed = NO;
    window.delegate = self;
    [window center];

    NSTextField *brand = label(@"Hello TSNet", 16, NSFontWeightBold, HTThemeAccentColor());
    self.tailnet = label(@"Tailscale", 12, NSFontWeightMedium, NSColor.secondaryLabelColor);
    NSSegmentedControl *tabs = [NSSegmentedControl segmentedControlWithLabels:@[@"设备", @"设置"]
        trackingMode:NSSegmentSwitchTrackingSelectOne target:self action:@selector(changePage:)];
    tabs.selectedSegment = 0;
    NSView *spacer = [NSView new];
    NSStackView *header = [NSStackView stackViewWithViews:@[brand, self.tailnet, spacer, tabs]];
    header.alignment = NSLayoutAttributeCenterY;
    header.spacing = 12;
    header.translatesAutoresizingMaskIntoConstraints = NO;
    NSBox *divider = [NSBox new];
    divider.boxType = NSBoxSeparator;
    divider.translatesAutoresizingMaskIntoConstraints = NO;

    self.devicePage = [self makeDevicePage];
    self.settingsPage = [self makeSettingsPage];
    self.settingsPage.hidden = YES;

    NSView *content = window.contentView;
    [content addSubview:header];
    [content addSubview:divider];
    [NSLayoutConstraint activateConstraints:@[
        [header.topAnchor constraintEqualToAnchor:content.topAnchor constant:14],
        [header.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:24],
        [header.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-24],
        [divider.topAnchor constraintEqualToAnchor:header.bottomAnchor constant:14],
        [divider.leadingAnchor constraintEqualToAnchor:content.leadingAnchor],
        [divider.trailingAnchor constraintEqualToAnchor:content.trailingAnchor]
    ]];
    for (NSView *page in @[self.devicePage, self.settingsPage]) {
        page.translatesAutoresizingMaskIntoConstraints = NO;
        [content addSubview:page];
        [NSLayoutConstraint activateConstraints:@[
            [page.topAnchor constraintEqualToAnchor:divider.bottomAnchor constant:22],
            [page.bottomAnchor constraintEqualToAnchor:content.bottomAnchor constant:-18],
            [page.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:24],
            [page.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-24]
        ]];
    }
    window.initialFirstResponder = self.search;
    return self;
}

- (NSView *)makeDevicePage {
    self.notice = [NSTextField wrappingLabelWithString:@"正在读取 Tailnet 设备…"];
    self.notice.textColor = NSColor.secondaryLabelColor;
    self.notice.font = [NSFont systemFontOfSize:12];

    self.search = [[NSSearchField alloc] initWithFrame:NSZeroRect];
    self.search.placeholderString = @"搜索设备名称、IP 或系统";
    self.search.delegate = self;
    self.search.sendsSearchStringImmediately = YES;
    NSButton *refresh = [NSButton buttonWithTitle:@"刷新设备" target:self action:@selector(refresh:)];
    NSStackView *toolbar = [NSStackView stackViewWithViews:@[self.search, refresh]];
    toolbar.alignment = NSLayoutAttributeCenterY;
    toolbar.spacing = 8;

    self.table = [[NSTableView alloc] initWithFrame:NSZeroRect];
    self.table.dataSource = self;
    self.table.delegate = self;
    self.table.selectionHighlightStyle = NSTableViewSelectionHighlightStyleNone;
    self.table.usesAlternatingRowBackgroundColors = YES;
    self.table.rowHeight = 57;
    self.table.intercellSpacing = NSMakeSize(8, 0);
    self.table.autoresizingMask = NSViewWidthSizable;
    NSArray *keys = @[@"number", @"name", @"addresses", @"os", @"state", @"latency"];
    NSArray *titles = @[@"序号", @"设备名称", @"Tailscale 地址", @"系统", @"状态", @"Ping"];
    NSArray *widths = @[@42, @205, @311, @80, @140, @80];
    NSArray *minimums = @[@42, @135, @294, @60, @124, @72];
    for (NSUInteger index = 0; index < keys.count; index++) {
        NSTableColumn *tableColumn = [[NSTableColumn alloc] initWithIdentifier:keys[index]];
        tableColumn.title = titles[index];
        tableColumn.width = [widths[index] doubleValue];
        tableColumn.minWidth = [minimums[index] doubleValue];
        if (index == 0) {
            tableColumn.maxWidth = tableColumn.minWidth;
            tableColumn.resizingMask = NSTableColumnNoResizing;
        }
        [self.table addTableColumn:tableColumn];
    }
    self.table.columnAutoresizingStyle = NSTableViewUniformColumnAutoresizingStyle;
    NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:NSZeroRect];
    scroll.hasVerticalScroller = YES;
    scroll.documentView = self.table;
    NSBox *tableCard = card(scroll, NSMakeSize(1, 1));

    self.deviceCount = label(@"0 台设备", 12, NSFontWeightRegular, NSColor.secondaryLabelColor);
    NSStackView *page = column(@[self.notice, toolbar, tableCard, self.deviceCount], 12);
    [NSLayoutConstraint activateConstraints:@[
        [self.notice.widthAnchor constraintEqualToAnchor:page.widthAnchor],
        [toolbar.widthAnchor constraintEqualToAnchor:page.widthAnchor],
        [tableCard.widthAnchor constraintEqualToAnchor:page.widthAnchor],
        [tableCard.heightAnchor constraintGreaterThanOrEqualToConstant:120]
    ]];
    return page;
}

- (NSView *)makeSettingsPage {
    self.accountAvatar = [[HTAccountAvatarView alloc] initWithFrame:NSZeroRect];
    [self.accountAvatar.widthAnchor constraintEqualToConstant:44].active = YES;
    [self.accountAvatar.heightAnchor constraintEqualToConstant:44].active = YES;
    self.accountName = label(@"正在读取账号…", 15, NSFontWeightSemibold, NSColor.labelColor);
    self.accountLogin = label(@"", 13, NSFontWeightRegular, NSColor.secondaryLabelColor);
    self.accountLogin.selectable = YES;
    NSStackView *identity = column(@[self.accountName, self.accountLogin], 4);
    self.logout = [NSButton buttonWithTitle:@"退出登录" target:self action:@selector(confirmLogout:)];
    self.logout.hasDestructiveAction = YES;
    NSStackView *accountRow = [NSStackView stackViewWithViews:@[self.accountAvatar, identity, [NSView new], self.logout]];
    accountRow.alignment = NSLayoutAttributeCenterY;
    accountRow.spacing = 12;
    NSBox *accountCard = card(accountRow, NSMakeSize(18, 16));
    NSTextField *helperTitle = label(@"路由助手", 15, NSFontWeightSemibold, NSColor.labelColor);
    self.routeHelperStatus = label(@"正在读取安装状态…", 13, NSFontWeightRegular, NSColor.secondaryLabelColor);
    self.routeHelperButton = [NSButton buttonWithTitle:@"安装路由助手" target:self action:@selector(manageRouteHelper:)];
    self.routeHelperButton.enabled = NO;
    self.routeHelperUpgradeButton = [NSButton buttonWithTitle:@"更新路由助手" target:self action:@selector(manageRouteHelper:)];
    self.routeHelperUpgradeButton.hidden = YES;
    NSStackView *helperDetails = column(@[helperTitle, self.routeHelperStatus], 6);
    NSStackView *helperRow = [NSStackView stackViewWithViews:@[helperDetails, [NSView new],
        self.routeHelperUpgradeButton, self.routeHelperButton]];
    helperRow.alignment = NSLayoutAttributeCenterY;
    helperRow.spacing = 12;
    NSTextField *helperDescription = [NSTextField wrappingLabelWithString:
        @"助手为 Tailscale 设备设置专用路由，通过 macOS 虚拟网卡（utun）把流量交给 Hello TSNet，让本机应用直接访问这些设备。"
         "新版助手还会同步本机的名称解析（hosts），让你用设备域名直接连接。"];
    helperDescription.font = [NSFont systemFontOfSize:12];
    helperDescription.textColor = NSColor.secondaryLabelColor;
    NSStackView *helperContent = column(@[helperRow, helperDescription], 8);
    [helperRow.widthAnchor constraintEqualToAnchor:helperContent.widthAnchor].active = YES;
    [helperDescription.widthAnchor constraintEqualToAnchor:helperContent.widthAnchor].active = YES;
    NSBox *helperCard = card(helperContent, NSMakeSize(18, 16));
    self.preferencesView = [[HTPreferencesView alloc] initWithFrame:NSZeroRect];
    NSBox *preferencesCard = card(self.preferencesView, NSMakeSize(18, 12));
    NSView *page = [NSView new];
    accountCard.translatesAutoresizingMaskIntoConstraints = NO;
    helperCard.translatesAutoresizingMaskIntoConstraints = NO;
    preferencesCard.translatesAutoresizingMaskIntoConstraints = NO;
    [page addSubview:accountCard];
    [page addSubview:helperCard];
    [page addSubview:preferencesCard];
    [NSLayoutConstraint activateConstraints:@[
        [accountCard.topAnchor constraintEqualToAnchor:page.topAnchor],
        [accountCard.leadingAnchor constraintEqualToAnchor:page.leadingAnchor],
        [accountCard.trailingAnchor constraintEqualToAnchor:page.trailingAnchor],
        [accountCard.heightAnchor constraintEqualToConstant:80],
        [helperCard.topAnchor constraintEqualToAnchor:accountCard.bottomAnchor constant:12],
        [helperCard.leadingAnchor constraintEqualToAnchor:page.leadingAnchor],
        [helperCard.trailingAnchor constraintEqualToAnchor:page.trailingAnchor],
        [preferencesCard.topAnchor constraintEqualToAnchor:helperCard.bottomAnchor constant:12],
        [preferencesCard.leadingAnchor constraintEqualToAnchor:page.leadingAnchor],
        [preferencesCard.trailingAnchor constraintEqualToAnchor:page.trailingAnchor],
        [preferencesCard.bottomAnchor constraintLessThanOrEqualToAnchor:page.bottomAnchor]
    ]];
    return page;
}

- (void)updateRouteHelperInstalled:(BOOL)installed upgrade:(BOOL)upgrade busy:(BOOL)busy {
    NSString *status = installed ? @"已安装" : @"未安装";
    if (upgrade) { status = @"已安装 · 更新后可通过主机名访问"; }
    self.routeHelperStatus.stringValue = busy ? @"等待操作完成…" : status;
    self.routeHelperButton.title = installed ? @"卸载路由助手" : @"安装路由助手";
    self.routeHelperButton.hasDestructiveAction = installed;
    self.routeHelperButton.enabled = !busy;
    self.routeHelperUpgradeButton.hidden = !upgrade;
    self.routeHelperUpgradeButton.enabled = !busy;
}

- (void)manageRouteHelper:(NSButton *)sender {
    HTRouteHelperAction action = sender == self.routeHelperUpgradeButton ? HTRouteHelperUpgrade
        : (sender.hasDestructiveAction ? HTRouteHelperRemove : HTRouteHelperInstall);
    if (self.routeHelperAction) { self.routeHelperAction(action); }
}

- (void)changePage:(NSSegmentedControl *)tabs {
    [self.window makeFirstResponder:tabs];
    self.settingsPage.hidden = tabs.selectedSegment == 0;
    self.devicePage.hidden = !self.settingsPage.hidden;
    if (!self.settingsPage.hidden) { [self.preferencesView refreshLoginItem]; }
    [self refreshAccountAvatar];
    [self setProbing:self.window.isKeyWindow && !self.devicePage.hidden];
}

- (void)setProbing:(BOOL)active {
    if (active == (self.probeSession != 0)) { return; }
    self.probeSession = goMenuSetProbing(active);
    self.latencies = [NSMutableDictionary dictionary];
    [self reloadLatencies];
}

- (void)windowDidBecomeKey:(NSNotification *)notification {
    if (!self.settingsPage.hidden) { [self.preferencesView refreshLoginItem]; }
    [self refreshAccountAvatar];
    [self setProbing:!self.devicePage.hidden];
}

- (void)windowDidResignKey:(NSNotification *)notification { [self setProbing:NO]; }

- (void)windowDidMiniaturize:(NSNotification *)notification { [self setProbing:NO]; }

- (void)reloadLatencies {
    NSInteger column = [self.table columnWithIdentifier:@"latency"];
    if (column >= 0 && self.visibleDevices.count) {
        [self.table reloadDataForRowIndexes:[NSIndexSet indexSetWithIndexesInRange:NSMakeRange(0, self.visibleDevices.count)]
            columnIndexes:[NSIndexSet indexSetWithIndex:column]];
    }
}

- (void)updateLatency:(NSDictionary *)result session:(unsigned long long)session {
    if (!self.probeSession || session != self.probeSession) { return; }
    for (NSDictionary *device in self.devices) {
        if ([device[@"id"] isEqual:result[@"id"]] && [device[@"state"] isEqualToString:@"online"] &&
            [device[@"addresses"] containsObject:result[@"address"]]) {
            self.latencies[result[@"id"]] = result;
            [self reloadLatencies];
            break;
        }
    }
}

- (void)show {
    [self showWindow:nil];
    [NSApp activateIgnoringOtherApps:YES];
    [self.window makeKeyAndOrderFront:nil];
    [self refresh:nil];
    if (!self.timer) {
        __weak HTDevicePanel *weakSelf = self;
        self.timer = [NSTimer scheduledTimerWithTimeInterval:3 repeats:YES block:^(NSTimer *timer) {
            [weakSelf refresh:nil];
        }];
    }
}

- (void)windowWillClose:(NSNotification *)notification {
    [self setProbing:NO];
    [self.accountAvatar loadPhotoURL:nil];
    [self.timer invalidate];
    self.timer = nil;
    [self.feedbackTimer invalidate];
    self.feedbackTimer = nil;
    [self.addressFeedback close];
}

- (void)windowDidResize:(NSNotification *)notification {
    [self.window.contentView layoutSubtreeIfNeeded];
    [self.table sizeToFit];
}

- (void)refresh:(id)sender { goMenuRefresh(); }

- (void)refreshAccountAvatar {
    if (self.window.isVisible && !self.settingsPage.hidden) {
        [self.accountAvatar loadPhotoURL:self.avatarAccount[@"profilePicURL"]];
    }
}

- (void)confirmLogout:(id)sender {
    NSAlert *alert = [NSAlert new];
    alert.messageText = @"退出 Tailscale 登录？";
    alert.informativeText = @"退出后，这台 Mac 与 Tailnet 设备的连接会断开。你可以从菜单栏重新授权登录。";
    [alert addButtonWithTitle:@"退出登录"].hasDestructiveAction = YES;
    [alert addButtonWithTitle:@"取消"].keyEquivalent = @"\033";
    [alert beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse response) {
        if (response == NSAlertFirstButtonReturn) {
            self.logout.enabled = NO;
            goMenuLogout();
        }
    }];
}

- (void)resetAccountAction { self.logout.enabled = YES; }

- (NSButton *)addressButton:(NSString *)address {
    NSButton *button = [NSButton buttonWithTitle:address target:self action:@selector(copyAddress:)];
    button.bordered = NO;
    button.cell.lineBreakMode = NSLineBreakByTruncatingMiddle;
    button.attributedTitle = [[NSAttributedString alloc] initWithString:address attributes:@{
        NSFontAttributeName: [NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightRegular],
        NSForegroundColorAttributeName: NSColor.labelColor
    }];
    NSImage *icon = [NSImage imageWithSystemSymbolName:@"document.on.document" accessibilityDescription:nil];
    button.image = [icon imageWithSymbolConfiguration:[NSImageSymbolConfiguration configurationWithPointSize:11 weight:NSFontWeightMedium]];
    button.imagePosition = NSImageTrailing;
    // Truncating button cells report a shortened intrinsic width. Prefer the
    // full title and copy icon, while allowing the table cell to constrain it.
    NSLayoutConstraint *fullWidth = [button.widthAnchor constraintGreaterThanOrEqualToConstant:
        ceil(button.attributedTitle.size.width + button.image.size.width + 8)];
    fullWidth.priority = NSLayoutPriorityDefaultHigh;
    fullWidth.active = YES;
    button.contentTintColor = NSColor.secondaryLabelColor;
    button.toolTip = [@"复制 " stringByAppendingString:address];
    [button setAccessibilityLabel:button.toolTip];
    return button;
}

- (void)update:(NSDictionary *)snapshot {
    NSDictionary *status = snapshot[@"status"];
    self.localDevice = status[@"localDevice"];
    NSDictionary *account = status[@"account"];
    NSString *network = account[@"tailnet"];
    self.tailnet.stringValue = network.length ? network : @"Tailscale";
    NSString *name = account[@"name"];
    NSString *login = account[@"login"];
    self.accountName.stringValue = name.length ? name : (login.length ? login : @"账号信息暂不可用");
    self.accountLogin.stringValue = name.length && ![name isEqualToString:login] ? login ?: @"" : @"";
    self.accountLogin.hidden = self.accountLogin.stringValue.length == 0;
    self.accountName.toolTip = self.accountName.stringValue;
    self.accountLogin.toolTip = self.accountLogin.stringValue;
    if (self.avatarAccount != account && ![self.avatarAccount isEqual:account]) {
        [self.accountAvatar loadPhotoURL:nil];
        self.avatarAccount = account;
    }
    [self refreshAccountAvatar];
    self.notice.stringValue = status[@"error"] ?: snapshot[@"deviceError"] ?: status[@"access"] ?: @"";
    self.notice.hidden = self.notice.stringValue.length == 0;
    if (snapshot[@"devices"]) { self.devices = snapshot[@"devices"]; }
    if (snapshot[@"deviceError"]) {
        NSMutableArray *unknown = [NSMutableArray arrayWithCapacity:self.devices.count];
        for (NSDictionary *device in self.devices) {
            NSMutableDictionary *copy = [device mutableCopy];
            copy[@"state"] = @"unknown";
            [unknown addObject:copy];
        }
        self.devices = unknown;
    }
    NSMutableDictionary *currentLatencies = [NSMutableDictionary dictionary];
    for (NSDictionary *device in self.devices) {
        NSDictionary *result = self.latencies[device[@"id"] ?: @""];
        if ([device[@"state"] isEqualToString:@"online"] && result &&
            [device[@"addresses"] containsObject:result[@"address"]]) {
            currentLatencies[device[@"id"]] = result;
        }
    }
    self.latencies = currentLatencies;
    [self filter];
}

- (void)filter {
    NSString *query = [self.search.stringValue stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
    NSMutableArray *matches = [NSMutableArray array];
    if (self.localDevice) { [matches addObject:self.localDevice]; }
    for (NSDictionary *device in self.devices) {
        NSString *haystack = [NSString stringWithFormat:@"%@ %@ %@ %@",
            device[@"name"] ?: @"", device[@"dnsName"] ?: @"", device[@"os"] ?: @"",
            [device[@"addresses"] componentsJoinedByString:@" "] ?: @""];
        if (query.length == 0 || [haystack localizedCaseInsensitiveContainsString:query]) { [matches addObject:device]; }
    }
    NSUInteger total = self.devices.count + (self.localDevice ? 1 : 0);
    self.deviceCount.stringValue = query.length == 0
        ? [NSString stringWithFormat:@"%lu 台设备", (unsigned long)matches.count]
        : [NSString stringWithFormat:@"%lu / %lu 台设备", (unsigned long)matches.count, (unsigned long)total];
    if (![matches isEqualToArray:self.visibleDevices]) {
        self.visibleDevices = matches;
        [self.table reloadData];
    }
}

- (void)controlTextDidChange:(NSNotification *)notification { [self filter]; }

- (NSInteger)numberOfRowsInTableView:(NSTableView *)tableView { return self.visibleDevices.count; }

- (NSTableRowView *)tableView:(NSTableView *)tableView rowViewForRow:(NSInteger)row {
    if (![self.visibleDevices[row][@"local"] boolValue]) { return nil; }
    NSTableRowView *view = [[NSTableRowView alloc] initWithFrame:NSZeroRect];
    view.backgroundColor = [NSColor.systemBlueColor colorWithAlphaComponent:0.10];
    return view;
}

- (id)tableView:(NSTableView *)tableView objectValueForTableColumn:(NSTableColumn *)tableColumn row:(NSInteger)row {
    NSDictionary *device = self.visibleDevices[row];
    if ([tableColumn.identifier isEqualToString:@"number"]) { return [NSString stringWithFormat:@"%ld", (long)row + 1]; }
    if ([tableColumn.identifier isEqualToString:@"latency"]) {
        if ([device[@"local"] boolValue] || ![device[@"state"] isEqualToString:@"online"]) { return @"—"; }
        NSDictionary *result = self.latencies[device[@"id"] ?: @""];
        if (![device[@"addresses"] containsObject:result[@"address"] ?: @""]) { result = nil; }
        if (result[@"milliseconds"]) { return [NSString stringWithFormat:@"%.1f ms", [result[@"milliseconds"] doubleValue]]; }
        if ([result[@"state"] isEqualToString:@"timeout"]) { return @"超时"; }
        if (result) { return @"不可达"; }
        return self.probeSession ? @"测量中…" : @"—";
    }
    if ([tableColumn.identifier isEqualToString:@"addresses"]) {
        return [device[@"addresses"] componentsJoinedByString:@"\n"] ?: @"";
    }
    if ([tableColumn.identifier isEqualToString:@"state"]) {
        return (@{@"online": @"在线", @"offline": @"离线", @"expired": @"授权已过期"})[device[@"state"]] ?: @"未知";
    }
    return device[tableColumn.identifier] ?: @"";
}

- (NSView *)tableView:(NSTableView *)tableView viewForTableColumn:(NSTableColumn *)tableColumn row:(NSInteger)row {
    NSDictionary *device = self.visibleDevices[row];
    NSString *value = [self tableView:tableView objectValueForTableColumn:tableColumn row:row];
    NSMutableArray<NSView *> *lines = [NSMutableArray array];
    if ([tableColumn.identifier isEqualToString:@"addresses"]) {
        for (NSString *address in device[@"addresses"]) {
            [lines addObject:[self addressButton:address]];
        }
    } else if ([tableColumn.identifier isEqualToString:@"name"]) {
        BOOL local = [device[@"local"] boolValue];
        NSTextField *name = label(value, 13, NSFontWeightSemibold, NSColor.labelColor);
        if (local) {
            name.toolTip = value;
            NSTextField *localLabel = label(@"本机", 10, NSFontWeightSemibold, NSColor.systemBlueColor);
            [localLabel setContentCompressionResistancePriority:NSLayoutPriorityRequired
                forOrientation:NSLayoutConstraintOrientationHorizontal];
            NSBox *badge = card(localLabel, NSMakeSize(5, 1));
            badge.cornerRadius = 4;
            badge.borderWidth = 0;
            badge.fillColor = [NSColor.systemBlueColor colorWithAlphaComponent:0.15];
            NSStackView *heading = [NSStackView stackViewWithViews:@[name, badge]];
            heading.alignment = NSLayoutAttributeCenterY;
            heading.spacing = 6;
            [lines addObject:heading];
        } else {
            [lines addObject:name];
        }
        NSString *dns = device[@"dnsName"];
        if (dns.length && ![dns isEqualToString:value]) {
            [lines addObject:[self addressButton:dns]];
            NSArray<NSString *> *parts = [dns componentsSeparatedByString:@"."];
            if (!local && parts.count == 4 && [dns.lowercaseString hasSuffix:@".ts.net"]) {
                [lines addObject:[self addressButton:[parts.firstObject stringByAppendingString:@".local"]]];
            }
        }
    } else if ([tableColumn.identifier isEqualToString:@"state"]) {
        NSString *state = device[@"state"];
        NSColor *color = [state isEqualToString:@"online"] ? NSColor.systemGreenColor :
            [state isEqualToString:@"expired"] ? NSColor.systemOrangeColor : NSColor.tertiaryLabelColor;
        NSImageView *dot = [NSImageView imageViewWithImage:[NSImage imageWithSystemSymbolName:@"circle.fill" accessibilityDescription:nil]];
        dot.symbolConfiguration = [NSImageSymbolConfiguration configurationWithPointSize:7 weight:NSFontWeightRegular];
        dot.contentTintColor = color;
        NSStackView *badge = [NSStackView stackViewWithViews:@[dot, label(value, 12, NSFontWeightMedium, NSColor.labelColor)]];
        badge.alignment = NSLayoutAttributeCenterY;
        badge.spacing = 5;
        [lines addObject:badge];
        if (![state isEqualToString:@"online"] && device[@"lastSeen"]) {
            NSString *seen = lastSeenTime(device[@"lastSeen"]);
            if (![seen isEqualToString:@"—"]) {
                [lines addObject:label(seen, 11, NSFontWeightRegular, NSColor.secondaryLabelColor)];
            }
        }
    } else if ([tableColumn.identifier isEqualToString:@"number"]) {
        [lines addObject:label(value, 11, NSFontWeightRegular, NSColor.secondaryLabelColor)];
    } else if (value.length) {
        [lines addObject:label(value, 12, NSFontWeightRegular, NSColor.labelColor)];
    }
    if (lines.count == 0) { [lines addObject:label(@"—", 12, NSFontWeightRegular, NSColor.secondaryLabelColor)]; }
    NSStackView *stack = column(lines, 3);
    stack.translatesAutoresizingMaskIntoConstraints = NO;
    NSView *cell = [NSView new];
    [cell addSubview:stack];
    [NSLayoutConstraint activateConstraints:@[
        [stack.leadingAnchor constraintEqualToAnchor:cell.leadingAnchor constant:6],
        [stack.trailingAnchor constraintLessThanOrEqualToAnchor:cell.trailingAnchor constant:-6],
        [stack.centerYAnchor constraintEqualToAnchor:cell.centerYAnchor]
    ]];
    for (NSView *line in lines) {
        [line.widthAnchor constraintLessThanOrEqualToAnchor:stack.widthAnchor].active = YES;
        if ([line isKindOfClass:NSTextField.class]) {
            NSTextField *field = (NSTextField *)line;
            field.toolTip = field.stringValue;
        }
    }
    return cell;
}

- (void)copyAddress:(NSButton *)button {
    NSPasteboard *pasteboard = NSPasteboard.generalPasteboard;
    [pasteboard clearContents];
    if (![pasteboard setString:button.title forType:NSPasteboardTypeString]) { return; }
    if (!self.addressFeedback) {
        self.addressFeedback = [NSPopover new];
        self.addressFeedback.behavior = NSPopoverBehaviorApplicationDefined;
        NSViewController *controller = [NSViewController new];
        NSTextField *message = label(@"✓ 已复制", 13, NSFontWeightSemibold, NSColor.labelColor);
        message.alignment = NSTextAlignmentCenter;
        controller.view = [NSView new];
        message.translatesAutoresizingMaskIntoConstraints = NO;
        [controller.view addSubview:message];
        [NSLayoutConstraint activateConstraints:@[
            [message.centerXAnchor constraintEqualToAnchor:controller.view.centerXAnchor],
            [message.centerYAnchor constraintEqualToAnchor:controller.view.centerYAnchor]
        ]];
        self.addressFeedback.contentViewController = controller;
        self.addressFeedback.contentSize = NSMakeSize(106, 40);
    }
    [self.addressFeedback showRelativeToRect:button.bounds ofView:button preferredEdge:NSRectEdgeMaxY];
    [self.feedbackTimer invalidate];
    __weak HTDevicePanel *weakSelf = self;
    self.feedbackTimer = [NSTimer scheduledTimerWithTimeInterval:2 repeats:NO block:^(NSTimer *timer) {
        [weakSelf.addressFeedback close];
    }];
}
@end
