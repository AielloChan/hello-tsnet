<div align="center">
  <img src="../macos/AppIcon.svg" width="128" height="128" alt="Logotipo de Hello TSNet">
  <h1>Hello TSNet</h1>
  <p>Accede a los dispositivos de tu Tailnet desde la barra de menús de macOS.</p>
  <p><a href="../README.md">English</a> · <a href="README.zh-CN.md">简体中文</a> · <a href="README.zh-TW.md">繁體中文</a> · <a href="README.ja.md">日本語</a> · Español</p>
</div>

Hello TSNet incorpora [Tailscale tsnet](https://tailscale.com/docs/features/tsnet) para unirse a tu Tailnet como nodo. Su panel de control nativo muestra las direcciones locales, los dispositivos visibles y el estado de la conexión.

- **Accede a los dispositivos de tu Tailnet.** El asistente de rutas opcional sincroniza las rutas de host IPv4 e IPv6. Usa la IP de Tailnet del dispositivo, su nombre completo `.ts.net` o su alias `.local` con el puerto original del servicio.
- **Comparte servicios locales con tu Tailnet.** Las conexiones TCP y UDP permitidas por la [política de acceso de tu Tailnet](https://tailscale.com/docs/features/access-control/grants) se reenvían al mismo puerto en `127.0.0.1`.
- **Gestiona la conexión.** Busca dispositivos, copia direcciones y nombres, inicia o cierra sesión y gestiona el asistente desde el panel nativo.

## Compilación e instalación

Necesitas macOS 15 o posterior, Go 1.27.1 o posterior, Python 3 y Xcode Command Line Tools. Clona este repositorio y ejecuta estos comandos en su directorio raíz.

```bash
./scripts/build-app.sh
python3 scripts/install-app.py
```

El instalador coloca la aplicación en `/Applications/Hello TSNet.app` y la abre. Selecciona **授权登录 (Iniciar sesión)** en la barra de menús y completa la autorización de Tailscale en el navegador.

Selecciona **安装路由助手… (Instalar asistente de rutas)** y aprueba la solicitud de administrador de macOS para acceder directamente por IP y por nombre. Puedes actualizar o desinstalar el asistente en **控制面板 → 设置 (Panel de control → Ajustes)**. Si instalaste un asistente anterior a la publicación del proyecto, desinstálalo desde la aplicación anterior antes de instalar la versión pública.

## Rutas y resolución de nombres

La aplicación se ejecuta con tu usuario. El asistente usa un socket Unix de launchd para configurar una interfaz virtual y las rutas de host de los dispositivos de la Tailnet. Guarda las asignaciones de nombres en un bloque marcado en `/etc/hosts`, respeta las entradas existentes y omite los alias `.local` ambiguos. Al salir de la aplicación o cerrar sesión, el asistente libera las rutas y elimina el bloque.

## Desarrollo

```bash
go test -race ./...
go vet ./...
go build ./cmd/hello-tsnet
./scripts/test-menubar.sh
```

Consulta las [notas de mantenimiento](HANDOFF.md) para conocer la arquitectura y las pruebas manuales de red.

## Licencia

[MIT](../LICENSE). Hello TSNet es un proyecto independiente que utiliza la biblioteca tsnet de Tailscale.
