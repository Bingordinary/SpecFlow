/**
 * SpecFlow plugin for OpenCode.ai
 *
 * Injects SpecFlow governance content (framework/concepts.md) via message transform.
 */

import path from 'path';
import fs from 'fs';

let _bootstrapCache = undefined;

export const SpecFlowPlugin = async ({ client, directory }) => {
  const getBootstrapContent = () => {
    if (_bootstrapCache !== undefined) return _bootstrapCache;

    const conceptsPath = path.resolve(directory, 'specflow/framework/concepts.md');
    if (!fs.existsSync(conceptsPath)) {
      _bootstrapCache = null;
      return null;
    }

    const conceptsContent = fs.readFileSync(conceptsPath, 'utf8');

    _bootstrapCache = `<SPECFLOW_CONCEPTS>
This project uses SpecFlow to manage design documents.

**Below is the full SpecFlow framework guide — read it carefully before starting work:**

${conceptsContent}
</SPECFLOW_CONCEPTS>`;

    return _bootstrapCache;
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
