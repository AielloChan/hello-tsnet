#import <AppKit/AppKit.h>

NSColor *HTThemeAccentColor(void);

@class HTPreferencesView;

typedef NS_ENUM(NSInteger, HTRouteHelperAction) {
    HTRouteHelperInstall, HTRouteHelperRemove, HTRouteHelperUpgrade
};

@interface HTDevicePanel : NSWindowController <NSWindowDelegate, NSTableViewDataSource, NSTableViewDelegate, NSSearchFieldDelegate>
@property(nonatomic, copy) void (^routeHelperAction)(HTRouteHelperAction action);
@property(nonatomic, strong, readonly) HTPreferencesView *preferencesView;
- (void)show;
- (void)update:(NSDictionary *)snapshot;
- (void)updateLatency:(NSDictionary *)result session:(unsigned long long)session;
- (void)updateRouteHelperInstalled:(BOOL)installed upgrade:(BOOL)upgrade busy:(BOOL)busy;
- (void)resetAccountAction;
@end
