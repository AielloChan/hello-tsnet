#import <AppKit/AppKit.h>

@interface HTPreferencesView : NSStackView
@property(nonatomic, copy) void (^menuBarColorAction)(NSColor *color);
- (void)updateMenuBarColor:(NSColor *)color;
- (void)refreshLoginItem;
@end
