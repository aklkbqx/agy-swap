//go:build darwin && cgo

#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>
#import <LocalAuthentication/LocalAuthentication.h>

static CFMutableDictionaryRef itemQuery(const char *service, const char *account) {
 CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
 CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
 if (!s || !a) { if(s) CFRelease(s); if(a) CFRelease(a); return NULL; }
 const void *keys[] = {kSecClass, kSecAttrService, kSecAttrAccount};
 const void *values[] = {kSecClassGenericPassword, s, a};
 CFMutableDictionaryRef query = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
 for (int i=0;i<3;i++) CFDictionarySetValue(query,keys[i],values[i]);
 CFRelease(a); CFRelease(s);
 return query;
}

int storeSecret(const char *service, const char *account, const void *secret, long size) {
 CFMutableDictionaryRef query = itemQuery(service, account);
 CFDataRef data = CFDataCreate(NULL, secret, size);
 if (!query || !data) { if(query) CFRelease(query); if(data) CFRelease(data); return 0; }
 LAContext *context = [[LAContext alloc] init];
 context.interactionNotAllowed = YES;
 CFDictionarySetValue(query, kSecUseAuthenticationContext, (CFTypeRef)context);
 const void *dataKey[] = {kSecValueData};
 const void *dataValue[] = {data};
 CFDictionaryRef update = CFDictionaryCreate(NULL, dataKey, dataValue, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
 OSStatus status = SecItemUpdate(query, update);
 if (status == errSecItemNotFound) { CFDictionarySetValue(query,kSecValueData,data); status = SecItemAdd(query,NULL); }
 [context release];
 CFRelease(update); CFRelease(query); CFRelease(data);
 return status == errSecSuccess;
}

// Reading in this process keeps the reader identical to the writer. A helper
// binary would carry a different code identity and fail the item's partition
// list, which no ACL entry can compensate for.
int loadSecret(const char *service, const char *account, void **out, long *size) {
 CFMutableDictionaryRef query = itemQuery(service, account);
 if (!query) return 0;
 LAContext *context = [[LAContext alloc] init];
 context.interactionNotAllowed = YES;
 CFDictionarySetValue(query, kSecUseAuthenticationContext, (CFTypeRef)context);
 CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
 CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
 CFTypeRef result = NULL;
 OSStatus status = SecItemCopyMatching(query, &result);
 [context release];
 CFRelease(query);
 if (status != errSecSuccess || !result) { if (result) CFRelease(result); return 0; }
 CFDataRef data = (CFDataRef)result;
 CFIndex length = CFDataGetLength(data);
 void *buffer = malloc(length > 0 ? length : 1);
 if (!buffer) { CFRelease(result); return 0; }
 if (length > 0) memcpy(buffer, CFDataGetBytePtr(data), length);
 CFRelease(result);
 *out = buffer;
 *size = length;
 return 1;
}

int deleteSecret(const char *service, const char *account) {
 CFMutableDictionaryRef query = itemQuery(service, account);
 if (!query) return 0;
 OSStatus status = SecItemDelete(query);
 CFRelease(query);
 return status == errSecSuccess || status == errSecItemNotFound;
}
