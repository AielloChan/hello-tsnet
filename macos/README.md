# macOS app

AppKit and the menu bar item run on the main thread. The Tailnet gateway runs in a Go goroutine. The native control panel reads state and device data in process. Quitting the app cancels the gateway context and closes the route session.

`./scripts/build-app.sh` builds the ad-hoc signed app at `dist/hello-tsnet.app`, including the route helper. Follow the [source build instructions](../README.md#build-from-source) to install the app. Run `./scripts/test-menubar.sh` to check the menu, control panel, and quit behavior.
