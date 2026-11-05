#ifndef HELLO_TSNET_MENU_H
#define HELLO_TSNET_MENU_H

enum HTMenuIconState { HTMenuIconIdle, HTMenuIconLoading, HTMenuIconReady };

void HTMenuRun(void);
void HTMenuUpdate(const char *json);
void HTMenuAccountFinished(const char *url, const char *errorMessage, int logoutResult);
void HTMenuRouteHelperFinished(const char *message, int removing);
void HTMenuFinish(const char *errorMessage);
void HTShowError(const char *errorMessage);

#endif
