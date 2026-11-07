#import "preferences.h"
#import "panel.h"
#import <ServiceManagement/ServiceManagement.h>

@interface HTPreferencesView ()
@property(nonatomic, strong) NSColorWell *menuBarColorWell;
@property(nonatomic, strong) NSButton *resetColorButton;
@property(nonatomic, strong) NSSwitch *loginItemSwitch;
@property(nonatomic, strong) NSTextField *loginItemStatus;
@property(nonatomic, strong) NSButton *loginItemsSettings;
@end

@implementation HTPreferencesView

- (instancetype)initWithFrame:(NSRect)frame {
    self = [super initWithFrame:frame];
    if (!self) { return nil; }

    self.orientation = NSUserInterfaceLayoutOrientationVertical;
    self.alignment = NSLayoutAttributeLeading;
    self.spacing = 10;
    self.menuBarColorWell = [[NSColorWell alloc] initWithFrame:NSZeroRect];
    self.menuBarColorWell.colorWellStyle = NSColorWellStyleMinimal;
    self.menuBarColorWell.supportsAlpha = NO;
    self.menuBarColorWell.continuous = YES;
    self.menuBarColorWell.target = self;
    self.menuBarColorWell.action = @selector(changeMenuBarColor:);
    [self.menuBarColorWell setAccessibilityLabel:@"菜单栏亮点颜色"];
    self.resetColorButton = [NSButton buttonWithTitle:@"恢复默认颜色" target:self action:@selector(resetMenuBarColor:)];
    NSStackView *color = [NSStackView stackViewWithViews:@[
        [NSTextField labelWithString:@"菜单栏亮点颜色"], [NSView new], self.resetColorButton, self.menuBarColorWell
    ]];
    color.alignment = NSLayoutAttributeCenterY;
    color.spacing = 12;

    self.loginItemSwitch = [[NSSwitch alloc] initWithFrame:NSZeroRect];
    self.loginItemSwitch.target = self;
    self.loginItemSwitch.action = @selector(changeLoginItem:);
    [self.loginItemSwitch setAccessibilityLabel:@"登录时启动"];
    self.loginItemStatus = [NSTextField labelWithString:@""];
    self.loginItemStatus.textColor = NSColor.secondaryLabelColor;
    self.loginItemsSettings = [NSButton buttonWithTitle:@"打开登录项设置" target:self action:@selector(openLoginItems:)];
    NSStackView *login = [NSStackView stackViewWithViews:@[
        [NSTextField labelWithString:@"登录时启动"], [NSView new], self.loginItemStatus, self.loginItemsSettings, self.loginItemSwitch
    ]];
    login.alignment = NSLayoutAttributeCenterY;
    login.spacing = 12;
    [self addArrangedSubview:color];
    [self addArrangedSubview:login];
    [color.widthAnchor constraintEqualToAnchor:self.widthAnchor].active = YES;
    [login.widthAnchor constraintEqualToAnchor:self.widthAnchor].active = YES;
    [self updateMenuBarColor:nil];
    [self refreshLoginItem];
    return self;
}

- (void)updateMenuBarColor:(NSColor *)color {
    self.menuBarColorWell.color = color ?: HTThemeAccentColor();
    self.resetColorButton.enabled = color != nil;
}

- (void)changeMenuBarColor:(NSColorWell *)sender {
    if (self.menuBarColorAction) { self.menuBarColorAction(sender.color); }
}

- (void)resetMenuBarColor:(id)sender {
    if (self.menuBarColorAction) { self.menuBarColorAction(nil); }
}

- (void)refreshLoginItem {
    SMAppServiceStatus status = SMAppService.mainAppService.status;
    BOOL pending = status == SMAppServiceStatusRequiresApproval;
    BOOL enabled = status == SMAppServiceStatusEnabled;
    self.loginItemSwitch.state = (enabled || pending) ? NSControlStateValueOn : NSControlStateValueOff;
    self.loginItemsSettings.hidden = !pending;
    self.loginItemStatus.stringValue = pending ? @"需在系统设置中允许" : (enabled ? @"已开启" : @"已关闭");
}

- (void)changeLoginItem:(NSSwitch *)sender {
    SMAppService *service = SMAppService.mainAppService;
    NSControlStateValue requested = sender.state;
    NSError *error = nil;
    BOOL succeeded = requested == NSControlStateValueOn ? [service registerAndReturnError:&error]
        : [service unregisterAndReturnError:&error];
    [self refreshLoginItem];
    if (!succeeded && self.loginItemSwitch.state != requested && self.window) {
        NSAlert *alert = [NSAlert new];
        alert.messageText = @"未能更新登录时启动";
        alert.informativeText = error.localizedDescription ?: @"macOS 无法更新登录项。";
        [alert addButtonWithTitle:@"好"];
        [alert beginSheetModalForWindow:self.window completionHandler:nil];
    }
}

- (void)openLoginItems:(id)sender { [SMAppService openSystemSettingsLoginItems]; }

@end
