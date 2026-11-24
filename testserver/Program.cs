// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

using Opc.Ua;
using Opc.Ua.Configuration;
using Opc.Ua.Server;
using System.Runtime.InteropServices;

namespace OpcUaTestServer;

class Program
{
    static async Task Main(string[] args)
    {
        Console.WriteLine("OPC UA Test Server starting...");

        var telemetry = DefaultTelemetry.Create(_ => { });
        var application = new ApplicationInstance(telemetry)
        {
            ApplicationName = "OPC UA Test Server",
            ApplicationType = ApplicationType.Server,
            ConfigSectionName = "OpcUaTestServer"
        };

        // Load configuration from XML
        var configPath = Path.Combine(AppContext.BaseDirectory, "server-config.xml");
        if (!File.Exists(configPath))
        {
            // Fallback: look next to the executable or in working dir
            configPath = Path.Combine(Directory.GetCurrentDirectory(), "server-config.xml");
        }

        if (File.Exists(configPath))
        {
            await application.LoadApplicationConfigurationAsync(configPath, silent: true);
        }
        else
        {
            await CreateDefaultConfigurationAsync(application);
        }

        // Check certificate
        bool haveAppCertificate = await application.CheckApplicationInstanceCertificatesAsync(silent: true);

        if (!haveAppCertificate)
        {
            Console.WriteLine("WARNING: No application certificate, using insecure connection.");
        }

        // Start the server
        var server = new TestServerInstance(telemetry);
        server.AddNodeManager(new TestNodeManagerFactory());
        await application.StartAsync(server);

        Console.WriteLine($"Server started. Endpoints:");
        foreach (var endpoint in server.GetEndpoints())
        {
            Console.WriteLine($"  {endpoint.EndpointUrl} [{endpoint.SecurityPolicyUri}]");
        }

        Console.WriteLine("Press Ctrl+C to exit...");

        // Wait for shutdown signal
        using var cts = new CancellationTokenSource();
        using var sigterm = OperatingSystem.IsWindows() ? null :
            PosixSignalRegistration.Create(PosixSignal.SIGTERM, context =>
            {
                context.Cancel = true;
                cts.Cancel();
            });
        Console.CancelKeyPress += (_, e) =>
        {
            e.Cancel = true;
            cts.Cancel();
        };

        try
        {
            await Task.Delay(Timeout.Infinite, cts.Token);
        }
        catch (OperationCanceledException)
        {
            // Expected on Ctrl+C
        }

        Console.WriteLine("Server shutting down...");
        await application.StopAsync();
    }

    private static async Task CreateDefaultConfigurationAsync(ApplicationInstance application)
    {
        var config = new ApplicationConfiguration
        {
            ApplicationName = "OPC UA Test Server",
            ApplicationUri = Utils.Format(@"urn:{0}:OpcUaTestServer", System.Net.Dns.GetHostName()),
            ApplicationType = ApplicationType.Server,
            SecurityConfiguration = new SecurityConfiguration
            {
                ApplicationCertificate = new CertificateIdentifier
                {
                    StoreType = @"Directory",
                    StorePath = @"pki/own",
                    SubjectName = "CN=OPC UA Test Server, O=OPC Foundation, DC=" + System.Net.Dns.GetHostName()
                },
                TrustedIssuerCertificates = new CertificateTrustList
                {
                    StoreType = @"Directory",
                    StorePath = @"pki/issuer"
                },
                TrustedPeerCertificates = new CertificateTrustList
                {
                    StoreType = @"Directory",
                    StorePath = @"pki/trusted"
                },
                RejectedCertificateStore = new CertificateTrustList
                {
                    StoreType = @"Directory",
                    StorePath = @"pki/rejected"
                },
                AutoAcceptUntrustedCertificates = true
            },
            TransportConfigurations = ArrayOf.Empty<TransportConfiguration>(),
            TransportQuotas = new TransportQuotas { OperationTimeout = 15000 },
            ServerConfiguration = new ServerConfiguration
            {
                BaseAddresses = ArrayOf.ToArrayOf(new[] { "opc.tcp://0.0.0.0:4840/TestServer" }),
                MinRequestThreadCount = 5,
                MaxRequestThreadCount = 100,
                MaxQueuedRequestCount = 200,
                SecurityPolicies = ArrayOf.ToArrayOf(new[] {
                    new ServerSecurityPolicy
                    {
                        SecurityMode = MessageSecurityMode.None,
                        SecurityPolicyUri = SecurityPolicies.None
                    }
                }),
                UserTokenPolicies = ArrayOf.ToArrayOf(new[] {
                    new UserTokenPolicy(UserTokenType.Anonymous)
                })
            }
        };

        await config.ValidateAsync(ApplicationType.Server);
        application.ApplicationConfiguration = config;
    }
}

/// <summary>
/// The OPC UA server instance that registers our custom node manager.
/// </summary>
class TestServerInstance : StandardServer
{
    public TestServerInstance(ITelemetryContext telemetry) : base(telemetry) { }

    protected override ServerProperties LoadServerProperties()
    {
        return new ServerProperties
        {
            ManufacturerName = "OPC UA Test Server",
            ProductName = "OPC UA Test Server for OpenTelemetry",
            ProductUri = "urn:opcua:testserver",
            SoftwareVersion = Utils.GetAssemblySoftwareVersion(),
            BuildNumber = "1.0.0",
            BuildDate = DateTime.UtcNow
        };
    }
}

class TestNodeManagerFactory : INodeManagerFactory
{
    public ArrayOf<string> NamespacesUris => ArrayOf.ToArrayOf(new[] { TestNodeManager.NamespaceUri });

    public INodeManager Create(IServerInternal server, ApplicationConfiguration configuration) =>
        new TestNodeManager(server, configuration);
}
