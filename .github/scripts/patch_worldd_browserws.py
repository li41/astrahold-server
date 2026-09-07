from pathlib import Path

path = Path("cmd/worldd/main.go")
text = path.read_text()


def replace_once(old: str, new: str, label: str) -> None:
    global text
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected exactly one match, got {count}")
    text = text.replace(old, new, 1)


replace_once(
    '\t"github.com/li41/astrahold-server/internal/codec/gamev1"\n',
    "",
    "remove legacy gamev1 import",
)

replace_once(
    '\t\ttcpAddress                       = flag.String("tcp", "127.0.0.1:7777", "Reliable TCP listen address")\n\t\tudpAddress                       = flag.String("udp", "127.0.0.1:7778", "Realtime UDP listen address")',
    '\t\tnetworkMode                      = flag.String("network-mode", worldNetworkTCPUDP, "Network adapter: tcpudp or browserws-dev (ephemeral loopback development/E2E)")\n\t\ttcpAddress                       = flag.String("tcp", "127.0.0.1:7777", "Reliable TCP listen address")\n\t\tudpAddress                       = flag.String("udp", "127.0.0.1:7778", "Realtime UDP listen address")\n\t\tbrowserWSAddress                 = flag.String("browser-ws", "127.0.0.1:7779", "Browser WebSocket listen address for browserws-dev mode")',
    "network flags",
)

replace_once(
    '\tflag.Parse()\n\tif err := validateRates(*tickRate, *snapshotRate); err != nil {',
    '\tflag.Parse()\n\tif err := validateWorldNetworkMode(*networkMode); err != nil {\n\t\tlog.Fatal(err)\n\t}\n\tif err := validateRates(*tickRate, *snapshotRate); err != nil {',
    "network mode validation",
)

replace_once(
    '\ttrustedTLSConfig, err := loadTrustedTLSIngressConfig(*trustedTLSListen, *trustedTLSCertFile, *trustedTLSKeyFile, *tcpAddress, trustedCharacterAuthenticator != nil)\n\tif err != nil {\n\t\tlog.Fatal(err)\n\t}',
    '\ttrustedTLSConfig, err := loadTrustedTLSIngressConfig(*trustedTLSListen, *trustedTLSCertFile, *trustedTLSKeyFile, *tcpAddress, trustedCharacterAuthenticator != nil)\n\tif err != nil {\n\t\tlog.Fatal(err)\n\t}\n\tif *networkMode == worldNetworkBrowserWSDev && (trustedCharacterAuthenticator != nil || trustedTLSConfig != nil) {\n\t\tlog.Fatal("browserws-dev is ephemeral local/E2E only and cannot enable trusted character authentication or trusted TLS ingress")\n\t}',
    "browserws trusted-auth fence",
)

old_network = """\tnetworkConfig := tcpudp.DefaultConfig()
\tnetworkConfig.TCPAddress = *tcpAddress
\tnetworkConfig.UDPAddress = *udpAddress
\tnetworkConfig.TickRateHz = uint16(*tickRate)
\tnetworkConfig.SnapshotRateHz = uint16(*snapshotRate)
\tnetworkConfig.WorldIdentity = worldIdentity
\tnetworkConfig.PlayerFactory = newWorldPlayerFactory(freshSpawn, loadedWorld.Definition.Agent)
\tnetworkConfig.CharacterRestoreFactory = characterStatePersistence.LoadRestore
\tif trustedCharacterAuthenticator != nil {
\t\tnetworkConfig.TrustedCharacterConnectionAuthenticator = trustedCharacterAuthenticator
\t}
\tserver := tcpudp.NewServer(networkConfig, runtime, gamev1.Codec{})
\tif trustedCharacterAuthRuntime != nil {
\t\tinitialScopes := activeTrustedCharacterAuthenticationScopes(trustedCharacterAuthRuntime.provider.snapshot(), time.Now().UTC())
\t\tserver.ReplaceTrustedCharacterAuthenticationScopes(initialScopes)
\t}
\tif err := server.Open(); err != nil {
\t\tlog.Fatal(err)
\t}
\tdefer server.Close()
"""
new_network = """\tplayerFactory := newWorldPlayerFactory(freshSpawn, loadedWorld.Definition.Agent)
\tnetwork, err := openWorldNetwork(worldNetworkConfig{
\t\tMode:                 *networkMode,
\t\tTCPAddress:           *tcpAddress,
\t\tUDPAddress:           *udpAddress,
\t\tBrowserWSAddress:     *browserWSAddress,
\t\tTickRateHz:           uint16(*tickRate),
\t\tSnapshotRateHz:       uint16(*snapshotRate),
\t\tWorldIdentity:        worldIdentity,
\t\tPlayerFactory:        playerFactory,
\t\tCharacterRestore:     characterStatePersistence.LoadRestore,
\t\tTrustedAuthenticator: trustedCharacterAuthenticator,
\t}, runtime)
\tif err != nil {
\t\tlog.Fatal(err)
\t}
\tdefer network.Close()
\tif trustedCharacterAuthRuntime != nil {
\t\tif network.tcp == nil {
\t\t\tlog.Fatal("trusted character authentication requires tcpudp network mode")
\t\t}
\t\tinitialScopes := activeTrustedCharacterAuthenticationScopes(trustedCharacterAuthRuntime.provider.snapshot(), time.Now().UTC())
\t\tnetwork.tcp.ReplaceTrustedCharacterAuthenticationScopes(initialScopes)
\t}
"""
replace_once(old_network, new_network, "network setup")

replace_once(
    "runTrustedCharacterAuthRuntime(ctx, reloadSignals, trustedCharacterAuthRuntime, server.ReplaceTrustedCharacterAuthenticationScopes, log.Printf)",
    "runTrustedCharacterAuthRuntime(ctx, reloadSignals, trustedCharacterAuthRuntime, network.tcp.ReplaceTrustedCharacterAuthenticationScopes, log.Printf)",
    "trusted auth reload target",
)

replace_once(
    "\tgo logNetworkErrors(ctx, server.Errors())",
    "\tif network.tcp != nil {\n\t\tgo logNetworkErrors(ctx, network.tcp.Errors())\n\t}",
    "network error logging",
)

replace_once(
    "tcp=%s udp=%s tick_rate=%dHz",
    "tcp=%s udp=%s browser_ws=%s network_mode=%s tick_rate=%dHz",
    "ready log network fields",
)
replace_once(
    "server.TCPAddr(), server.UDPAddr(), *tickRate, *snapshotRate",
    "network.TCPAddr(), network.UDPAddr(), network.BrowserWSAddr(), network.Mode(), *tickRate, *snapshotRate",
    "ready log network args",
)

replace_once(
    "\tif err := server.Serve(ctx); err != nil {",
    '\tif network.browser != nil {\n\t\tlog.Printf("browser WebSocket development adapter: enabled=true listen=%s ephemeral_identity=true loopback_only=true authoritative_runtime=shared", network.BrowserWSAddr())\n\t}\n\tif err := network.Serve(ctx); err != nil {',
    "network serve",
)

path.write_text(text)
