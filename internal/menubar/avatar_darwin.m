#import "avatar.h"
#import <ImageIO/ImageIO.h>
#import <QuartzCore/QuartzCore.h>

static const NSUInteger maxAvatarBytes = 1024 * 1024;

static BOOL isAvatarURL(NSURL *url) {
    return [url.scheme.lowercaseString isEqualToString:@"https"] && url.host.length && !url.user && !url.password;
}

static CGImageRef copyAvatarThumbnail(NSData *data) {
    CGImageSourceRef source = CGImageSourceCreateWithData((__bridge CFDataRef)data, NULL);
    if (!source) { return NULL; }
    NSDictionary *properties = CFBridgingRelease(CGImageSourceCopyPropertiesAtIndex(source, 0, NULL));
    NSUInteger width = [properties[(id)kCGImagePropertyPixelWidth] unsignedIntegerValue];
    NSUInteger height = [properties[(id)kCGImagePropertyPixelHeight] unsignedIntegerValue];
    CGImageRef thumbnail = NULL;
    if (width && height && width <= 4096 && height <= 4096) {
        NSDictionary *options = @{ (id)kCGImageSourceCreateThumbnailFromImageAlways: @YES,
            (id)kCGImageSourceCreateThumbnailWithTransform: @YES, (id)kCGImageSourceThumbnailMaxPixelSize: @128 };
        thumbnail = CGImageSourceCreateThumbnailAtIndex(source, 0, (__bridge CFDictionaryRef)options);
    }
    CFRelease(source);
    if (!thumbnail) { return NULL; }
    size_t thumbnailWidth = CGImageGetWidth(thumbnail);
    size_t thumbnailHeight = CGImageGetHeight(thumbnail);
    size_t side = MIN(thumbnailWidth, thumbnailHeight);
    CGRect crop = CGRectMake((thumbnailWidth - side) / 2, (thumbnailHeight - side) / 2, side, side);
    CGImageRef square = CGImageCreateWithImageInRect(thumbnail, crop);
    CGImageRelease(thumbnail);
    return square;
}

@interface HTAccountAvatarView () <NSURLSessionDataDelegate>
@property(nonatomic, strong) NSImage *placeholder;
@property(nonatomic, strong) NSURL *photoURL;
@property(nonatomic, strong) NSURLSessionDataTask *photoTask;
@property(nonatomic, strong) NSMutableData *photoData;
@end

@implementation HTAccountAvatarView

- (instancetype)initWithFrame:(NSRect)frame {
    self = [super initWithFrame:frame];
    if (!self) { return nil; }
    self.placeholder = [NSImage imageWithSystemSymbolName:@"person.crop.circle.fill" accessibilityDescription:nil];
    self.image = self.placeholder;
    self.imageScaling = NSImageScaleProportionallyUpOrDown;
    self.symbolConfiguration = [NSImageSymbolConfiguration configurationWithPointSize:44 weight:NSFontWeightRegular];
    self.contentTintColor = NSColor.secondaryLabelColor;
    self.wantsLayer = YES;
    self.layer.cornerRadius = 22;
    self.layer.masksToBounds = YES;
    [self setAccessibilityLabel:@"账号头像"];
    return self;
}

- (NSEdgeInsets)alignmentRectInsets {
    // SF Symbols have alignment insets. Keep the avatar's clipping frame square.
    return NSEdgeInsetsMake(0, 0, 0, 0);
}

+ (NSURLSessionConfiguration *)photoSessionConfiguration {
    NSURLSessionConfiguration *configuration = NSURLSessionConfiguration.ephemeralSessionConfiguration;
    configuration.HTTPShouldSetCookies = NO;
    configuration.URLCredentialStorage = nil;
    configuration.timeoutIntervalForRequest = 8;
    configuration.timeoutIntervalForResource = 8;
    return configuration;
}

- (void)loadPhotoURL:(NSString *)value {
    NSURL *url = value.length ? [NSURL URLWithString:value] : nil;
    if (!isAvatarURL(url)) { url = nil; }
    if (self.photoURL == url || [self.photoURL isEqual:url]) { return; }

    [self.photoTask cancel];
    self.photoTask = nil;
    self.photoData = nil;
    self.photoURL = url;
    self.image = self.placeholder;
    if (!url) { return; }

    NSURLSession *session = [NSURLSession sessionWithConfiguration:[self.class photoSessionConfiguration]
        delegate:self delegateQueue:NSOperationQueue.mainQueue];
    self.photoData = [NSMutableData data];
    self.photoTask = [session dataTaskWithURL:url];
    [self.photoTask resume];
    [session finishTasksAndInvalidate];
}

- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task willPerformHTTPRedirection:(NSHTTPURLResponse *)response
    newRequest:(NSURLRequest *)request completionHandler:(void (^)(NSURLRequest *))completionHandler {
    completionHandler(task == self.photoTask && isAvatarURL(request.URL) ? request : nil);
}

- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task didReceiveResponse:(NSURLResponse *)response
    completionHandler:(void (^)(NSURLSessionResponseDisposition))completionHandler {
    NSInteger status = [response isKindOfClass:NSHTTPURLResponse.class] ? ((NSHTTPURLResponse *)response).statusCode : 0;
    BOOL accepted = task == self.photoTask && status >= 200 && status < 300 && response.expectedContentLength <= (int64_t)maxAvatarBytes;
    completionHandler(accepted ? NSURLSessionResponseAllow : NSURLSessionResponseCancel);
}

- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task didReceiveData:(NSData *)data {
    if (task != self.photoTask) { return; }
    if (data.length > maxAvatarBytes - self.photoData.length) {
        self.photoData = nil;
        self.photoTask = nil;
        [task cancel];
        return;
    }
    [self.photoData appendData:data];
}

- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task didCompleteWithError:(NSError *)error {
    if (task != self.photoTask) { return; }
    NSData *data = self.photoData;
    self.photoData = nil;
    if (error || !data.length) {
        self.photoTask = nil;
        return;
    }
    __weak HTAccountAvatarView *weakSelf = self;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
        CGImageRef thumbnail = copyAvatarThumbnail(data);
        dispatch_async(dispatch_get_main_queue(), ^{
            HTAccountAvatarView *view = weakSelf;
            if (task == view.photoTask) {
                if (thumbnail) { view.image = [[NSImage alloc] initWithCGImage:thumbnail size:NSZeroSize]; }
                view.photoTask = nil;
            }
            if (thumbnail) { CGImageRelease(thumbnail); }
        });
    });
}

@end
