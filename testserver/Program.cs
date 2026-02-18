// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

using Opc.Ua;
using Opc.Ua.Configuration;
using Opc.Ua.Server;

namespace OpcUaTestServer;

class Program
{
    static async Task Main(string[] args)
    {
        Console.WriteLine("OPC UA Test Server starting...");

        var application = new ApplicationInstance
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

        ApplicationConfiguration config;
        if (File.Exists(configPath))
        {
            config = await application.LoadApplicationConfiguration(configPath, silent: true);
        }
        else
        {
            config = CreateDefaultConfiguration(application);
        }

        // Check certificate
        bool haveAppCertificate = await application.CheckApplicationInstanceCertificate(
            silent: true, minimumKeySize: 0);

        if (!haveAppCertificate)
        {
            Console.WriteLine("WARNING: No application certificate, using insecure connection.");
        }

        // Auto-accept untrusted certificates for testing
        config.CertificateValidator.CertificateValidation += (sender, e) =>
        {
            e.Accept = true;
        };

        // Start the server
        var server = new TestServerInstance();
        await application.Start(server);

        Console.WriteLine($"Server started. Endpoints:");
        foreach (var endpoint in server.GetEndpoints())
        {
            Console.WriteLine($"  {endpoint.EndpointUrl} [{endpoint.SecurityPolicyUri}]");
        }

        Console.WriteLine("Press Ctrl+C to exit...");

        // Wait for shutdown signal
        var cts = new CancellationTokenSource();
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
        server.Stop();
    }

    private static ApplicationConfiguration CreateDefaultConfiguration(ApplicationInstance application)
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
            TransportConfigurations = new TransportConfigurationCollection(),
            TransportQuotas = new TransportQuotas { OperationTimeout = 15000 },
            ServerConfiguration = new ServerConfiguration
            {
                BaseAddresses = { "opc.tcp://0.0.0.0:4840/TestServer" },
                MinRequestThreadCount = 5,
                MaxRequestThreadCount = 100,
                MaxQueuedRequestCount = 200,
                SecurityPolicies = new ServerSecurityPolicyCollection
                {
                    new ServerSecurityPolicy
                    {
                        SecurityMode = MessageSecurityMode.None,
                        SecurityPolicyUri = SecurityPolicies.None
                    }
                },
                UserTokenPolicies = new UserTokenPolicyCollection
                {
                    new UserTokenPolicy(UserTokenType.Anonymous)
                }
            }
        };

        config.Validate(ApplicationType.Server).GetAwaiter().GetResult();
        application.ApplicationConfiguration = config;
        return config;
    }
}

/// <summary>
/// The OPC UA server instance that registers our custom node manager.
/// </summary>
class TestServerInstance : StandardServer
{
    protected override MasterNodeManager CreateMasterNodeManager(IServerInternal server, ApplicationConfiguration configuration)
    {
        Console.WriteLine("Creating node managers...");

        var nodeManagers = new List<INodeManager>
        {
            new TestNodeManager(server, configuration)
        };

        return new MasterNodeManager(server, configuration, null, nodeManagers.ToArray());
    }

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
