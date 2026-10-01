/**
 * SpecFlow plugin for OpenCode.ai
 *
 * Injects the SpecFlow session bootstrap (framework/concepts.md) into the
 * system context of every agent-loop model request. The bootstrap is read
 * fresh on each request — no module-level cache — so a framework content
 * change takes effect without a process restart and multiple project
 * directories can never share stale content
 * (framework/hooks.md §Adapter Injection Contract).
 *
 * This targets the OpenCode V2 plugin API: a default-exported definition with
 * a stable `id` and a `setup(ctx)` function that registers hooks through the
 * plugin context. V1 plugins never load in V2.
 */

import path from 'path';
import fs from 'fs';

export default {
  id: 'specflow',
  async setup(ctx) {
    const getBootstrapContent = () => {
      const conceptsPath = path.resolve(ctx.location.directory, 'specflow/framework/concepts.md');
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

    await ctx.session.hook('context', (event) => {
      const bootstrap = getBootstrapContent();
      if (!bootstrap) return;

      event.system.push({ type: 'text', text: bootstrap });
    });
  },
};
