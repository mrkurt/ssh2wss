import * as vscode from 'vscode';

// Create output channel for logging
let outputChannel: vscode.OutputChannel;

function log(message: string) {
	const timestamp = new Date().toISOString();
	outputChannel.appendLine(`[${timestamp}] ${message}`);
}

export function activate(context: vscode.ExtensionContext) {
	// Initialize output channel
	outputChannel = vscode.window.createOutputChannel('Fly Dev');
	context.subscriptions.push(outputChannel);

	log('Fly Dev extension activated');

	// Register error handler
	const errorHandler = vscode.window.registerUriHandler({
		handleUri(uri: vscode.Uri): void {
			if (uri.path.includes('remote-ssh-error')) {
				log('Intercepted Remote SSH error URI');
				handleSSHError();
				return;
			}
		}
	});
	context.subscriptions.push(errorHandler);

	// Monitor diagnostic errors
	context.subscriptions.push(
		vscode.languages.onDidChangeDiagnostics(e => {
			for (const uri of e.uris) {
				const diagnostics = vscode.languages.getDiagnostics(uri);
				for (const diagnostic of diagnostics) {
					if (diagnostic.message.includes('Could not establish connection') || 
						diagnostic.message.includes('ssh: connect to host')) {
						log(`Intercepted SSH error: ${diagnostic.message}`);
						handleSSHError();
						return;
					}
				}
			}
		})
	);

	// Monitor error output
	const outputHandler = vscode.workspace.onDidChangeTextDocument(e => {
		if (e.document.uri.scheme === 'output' && e.document.uri.path.includes('Remote')) {
			const text = e.document.getText();
			if (text.includes('Could not establish connection') || 
				text.includes('ssh: connect to host')) {
				log(`Intercepted SSH error in output: ${text}`);
				handleSSHError();
			}
		}
	});
	context.subscriptions.push(outputHandler);

	async function handleSSHError() {
		const troubleshoot = await vscode.window.showErrorMessage(
			'Unable to connect to Fly Dev environment',
			{
				modal: true,
				detail: 'This could be because:\n• The SSH server is not running\n• Port 2222 is not accessible\n• The dev user is not configured correctly',
			},
			'Troubleshoot',
			'Try Again'
		);

		if (troubleshoot === 'Try Again') {
			log('User requested retry');
			await vscode.commands.executeCommand('vscode-fly-dev.setupDev');
		} else if (troubleshoot === 'Troubleshoot') {
			log('User requested troubleshooting');
			// TODO: Add your troubleshooting workflow here
			await vscode.window.showInformationMessage('Opening troubleshooting guide...');
		}
	}

	let disposable = vscode.commands.registerCommand('vscode-fly-dev.setupDev', async () => {
		log('Starting Fly Dev setup');

		// Get test configuration input
		const configInput = await vscode.window.showInputBox({
			title: 'Fly Dev Configuration',
			prompt: 'Enter your test configuration/credential info',
			placeHolder: 'e.g., your-test-config'
		});

		if (!configInput) {
			log('Setup cancelled by user');
			vscode.window.showInformationMessage('Setup cancelled');
			return;
		}

		log(`Received configuration: ${configInput}`);

		try {
			// Add SSH config for localhost:2222
			const sshConfig = `Host fly-dev-local
	HostName localhost
	User dev
	Port 2222`;

			log('Writing SSH configuration');
			await vscode.workspace.fs.writeFile(
				vscode.Uri.file(`${process.env.HOME}/.ssh/config`),
				Buffer.from(sshConfig + '\n', 'utf8')
			);
			log('SSH configuration written successfully');

			// Start connection immediately
			try {
				log('Initiating connection to development environment');
				// Start connection with progress indicator
				await vscode.window.withProgress({
					location: vscode.ProgressLocation.Notification,
					title: "Connecting to development environment...",
					cancellable: true
				}, async (progress, token) => {
					token.onCancellationRequested(() => {
						log('Connection cancelled by user');
						vscode.window.showInformationMessage('Connection cancelled');
					});

					progress.report({ increment: 30, message: "Establishing SSH connection..." });
					log('Attempting SSH connection');
					
					// Attempt connection
					await vscode.commands.executeCommand(
						'vscode.openFolder',
						vscode.Uri.parse(`vscode-remote://ssh-remote+localhost/home/dev`),
						{ forceNewWindow: false }
					);

					// Add a small delay to catch immediate failures
					await new Promise(resolve => setTimeout(resolve, 2000));
					
					// Check if we're connected
					if (!vscode.env.remoteName) {
						log('Connection failed - no remote name found');
						throw new Error('Failed to establish connection');
					}
					log('Connection successful');
				});
			} catch (connError) {
				log(`Connection error: ${connError}`);
				await handleSSHError();
			}
		} catch (error) {
			log(`Configuration error: ${error}`);
			vscode.window.showErrorMessage(`Failed to configure SSH: ${error}`);
		}
	});

	context.subscriptions.push(disposable);
}