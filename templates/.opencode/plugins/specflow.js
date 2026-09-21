/**
 * SpecFlow plugin for OpenCode.ai
 *
 * Injects the SpecFlow session bootstrap (framework/concepts.md) via message
 * transform. The bootstrap is read fresh at every transform — no module-level
 * cache — so a framework content change takes effect without a process restart
 * and multiple project directories can never share stale content
 * (framework/hooks.md §Adapter Injection Contract).
 */

import path from 'path';
import fs from 'fs';

export const SpecFlowPlugin = async ({ client, directory }) => {
  const getBootstrapContent = () => {
    const conceptsPath = path.resolve(directory, 'specflow/framework/concepts.md');
    if (!fs.existsSync(conceptsPath)) {
      return null;
    }

    const conceptsContent = fs.readFileSync(conceptsPath, 'utf8');

    return `<SPECFLOW_CONCEPTS>
This project uses SpecFlow to manage design documents.

**Below is the SpecFlow session bootstrap — read it before starting work. It states the operating rules and routes each supported trigger to its command package, which you read on demand:**

${conceptsContent}
</SPECFLOW_CONCEPTS>`;
  };

  return {
    'experimental.chat.messages.transform': async (_input, output) => {
      const bootstrap = getBootstrapContent();
      if (!bootstrap || !output.messages.length) return;

      const firstUser = output.messages.find(m => m.info.role === 'user');
      if (!firstUser || !firstUser.parts.length) return;

      if (firstUser.parts.some(p => p.type === 'text' && p.text.includes('SPECFLOW_CONCEPTS'))) return;

      const ref = firstUser.parts[0];
      firstUser.parts.unshift({ ...ref, type: 'text', text: bootstrap });
    }
  };
};
