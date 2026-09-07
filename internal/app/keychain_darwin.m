//go:build darwin && cgo

#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#import <LocalAuthentication/LocalAuthentication.h>

int storeSecret(const char *service, const char *account, const void *secret, long size) {
 CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
 CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
 CFDataRef data = CFDataCreate(NULL, secret, size);
 if (!s || !a || !data) { if(s) CFRelease(s); if(a) CFRelease(a); if(data) CFRelease(data); return 0; }
 LAContext *context = [[LAContext alloc] init];
 context.interactionNotAllowed = YES;
 const void *keys[] = {kSecClass, kSecAttrService, kSecAttrAccount, kSecUseAuthenticationContext};
 const void *values[] = {kSecClassGenericPassword, s, a, (CFTypeRef)context};
 CFMutableDictionaryRef query = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
 for (int i=0;i<4;i++) CFDictionarySetValue(query,keys[i],values[i]);
 const void *dataKey[] = {kSecValueData};
 const void *dataValue[] = {data};
 CFDictionaryRef update = CFDictionaryCreate(NULL, dataKey, dataValue, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
 OSStatus status = SecItemUpdate(query, update);
 if (status == errSecItemNotFound) { CFDictionarySetValue(query,kSecValueData,data); status = SecItemAdd(query,NULL); }
 [context release];
 CFRelease(update); CFRelease(query); CFRelease(data); CFRelease(a); CFRelease(s);
 return status == errSecSuccess;
}
