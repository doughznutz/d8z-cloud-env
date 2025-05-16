/*--------------------------------------------------------------------------------------
 *  Copyright 2025 Glass Devtools, Inc. All rights reserved.
 *  Licensed under the Apache License, Version 2.0. See LICENSE.txt for more information.
 *--------------------------------------------------------------------------------------*/

import { Disposable } from '../../../../base/common/lifecycle.js';
import { IWorkbenchContribution, WorkbenchPhase, registerWorkbenchContribution2 } from '../../../common/contributions.js';
import { IVoidSettingsService } from '../common/voidSettingsService.js';
import { ILogService } from '../../../../platform/log/common/log.js';
import { ProviderName } from '../common/voidSettingsTypes.js'; // Import the type

/**
 * IMPORTANT: How to provide the API key?
 * Avoid hardcoding the key here! Use a secure method like environment variables.
 * Example using an environment variable `VOID_OPENAI_API_KEY`:
 * const apiKeyToInject = process.env.VOID_OPENAI_API_KEY;
 *
 * You'll need to ensure this environment variable is available when Void Editor runs.
 */
const PROVIDER_TO_INJECT: ProviderName = 'openAI'; // CHANGE THIS to the provider you want to inject for (e.g., 'anthropic')
const API_KEY_ENV_VAR = 'VOID_OPENAI_API_KEY'; // CHANGE THIS to your environment variable name

export class VoidApiKeyInjectorContribution extends Disposable implements IWorkbenchContribution {

	static readonly ID = 'workbench.contrib.voidApiKeyInjector';

	constructor(
		@IVoidSettingsService private readonly voidSettingsService: IVoidSettingsService,
		@ILogService private readonly logService: ILogService
	) {
		super();
		this.logService.info('[VoidApiKeyInjector] Contribution constructor called.');
		this.injectApiKey();
	}

	private async injectApiKey(): Promise<void>; {
		// *** IMPORTANT: Replace this with your secure method of getting the API key ***
		const apiKeyToInject = process.env[API_KEY_ENV_VAR];
		// *****************************************************************************

		if (!apiKeyToInject) {
			this.logService.info(`[VoidApiKeyInjector] No API key found in environment variable "${API_KEY_ENV_VAR}" for provider "${PROVIDER_TO_INJECT}". Skipping injection.`);
			return;
		}

		if (!PROVIDER_TO_INJECT) {
			this.logService.error('[VoidApiKeyInjector] PROVIDER_TO_INJECT is not set in the code. Cannot inject key.');
			return;
		}

		try {
			this.logService.info(`[VoidApiKeyInjector] Waiting for VoidSettingsService initialization for provider "${PROVIDER_TO_INJECT}"...`);
			await this.voidSettingsService.waitForInitState;
			this.logService.info(`[VoidApiKeyInjector] VoidSettingsService initialized. Injecting API key for provider "${PROVIDER_TO_INJECT}".`);

			// Set the API key for the specified provider
			await this.voidSettingsService.setSettingOfProvider(PROVIDER_TO_INJECT, 'apiKey', apiKeyToInject);

			this.logService.info(`[VoidApiKeyInjector] API key for "${PROVIDER_TO_INJECT}" injected successfully.`);

		} catch (error) {
			this.logService.error(`[VoidApiKeyInjector] Error injecting API key for "${PROVIDER_TO_INJECT}":`, error);
		}
	}
}

// Register the contribution to run on startup (WorkbenchPhase.Eventually is usually safe)
registerWorkbenchContribution2(
	VoidApiKeyInjectorContribution.ID,
	VoidApiKeyInjectorContribution,
	WorkbenchPhase.Eventually // Run after most other things are initialized
);

