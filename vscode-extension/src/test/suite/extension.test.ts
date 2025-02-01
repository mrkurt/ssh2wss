import * as assert from 'node:assert';
import * as vscode from 'vscode';
import { after } from 'mocha';

suite('FlySsh Extension Test Suite', () => {
	after(() => {
		vscode.window.showInformationMessage('All tests done!');
	});

	test('Extension should be present', () => {
		assert.ok(vscode.extensions.getExtension('flyssh'));
	});

	test('Should register flyssh.configure command', async () => {
		const commands = await vscode.commands.getCommands();
		assert.ok(commands.includes('flyssh.configure'));
	});

	test('Should show welcome walkthrough', () => {
		const extension = vscode.extensions.getExtension('flyssh');
		const packageJSON = extension?.packageJSON;
		assert.ok(packageJSON.walkthroughs);
		assert.strictEqual(packageJSON.walkthroughs[0].id, 'flyssh.setup');
	});
}); 
