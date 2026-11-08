import { defineConfig, devices } from '@playwright/test';
import { STUB_PORT } from './e2e/a11y/ports.ts';

// The accessibility check runs against the BUILT app (`pnpm build` first) and a loopback
// stub API, so it needs no backend, no secrets and no network. `playwright.config.ts`
// keeps meaning "against the real stack on :8090" and ignores this spec.
const APP_PORT = 18090;

export default defineConfig({
	testDir: 'e2e',
	testMatch: 'a11y.spec.ts',
	forbidOnly: !!process.env.CI,
	retries: 0,
	reporter: 'list',
	use: {
		baseURL: `http://127.0.0.1:${APP_PORT}`,
		colorScheme: 'light'
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
	webServer: [
		{
			command: 'node e2e/a11y/stub-api.ts',
			url: `http://127.0.0.1:${STUB_PORT}/api/v1/plans`,
			env: { A11Y_STUB_PORT: String(STUB_PORT) },
			reuseExistingServer: false,
			stdout: 'pipe',
			stderr: 'pipe'
		},
		{
			command: 'node build',
			url: `http://127.0.0.1:${APP_PORT}/pricing`,
			env: {
				HOST: '127.0.0.1',
				PORT: String(APP_PORT),
				API_INTERNAL_URL: `http://127.0.0.1:${STUB_PORT}`
			},
			reuseExistingServer: false,
			stdout: 'pipe',
			stderr: 'pipe'
		}
	]
});
