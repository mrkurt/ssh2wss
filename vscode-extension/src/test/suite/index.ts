import * as path from 'node:path';
import Mocha from 'mocha';
import glob from 'glob';

export function run(): Promise<void> {
	// Create the mocha test
	const mocha = new Mocha({
		ui: 'tdd',
		color: true
	});

	const testsRoot = path.resolve(__dirname, '.');

	return new Promise<void>((resolve, reject) => {
		glob.sync('**/**.test.js', { cwd: testsRoot }).forEach(f => {
			mocha.addFile(path.resolve(testsRoot, f));
		});

		try {
			// Run the mocha test
			mocha.run((failures: number) => {
				if (failures > 0) {
					reject(new Error(`${failures} tests failed.`));
				} else {
					resolve();
				}
			});
		} catch (err) {
			console.error(err);
			reject(err);
		}
	});
} 
