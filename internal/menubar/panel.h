#import <AppKit/AppKit.h>

NSColor *HTThemeAccentColor(void);

typedef NS_ENUM(NSInteger, HTRouteHelperAction) {
    HTRouteHelperInstall, HTRouteHelperRemove, HTRouteHelperUpgrade
};

@interface HTDevicePanel : NSWindowController <NSWindowDelegate, NSTableViewDataSource, NSTableViewDelegate, NSSearchFieldDelegate>
@property(nonatomic, copy) void (^routeHelperAction)(HTRouteHelperAction action);
- (void)show;
- (void)update:(NSDictionary *)snapshot;
- (void)updateRouteHelperInstalled:(BOOL)installed upgrade:(BOOL)upgrade busy:(BOOL)busy;
- (void)resetAccountAction;
@end
