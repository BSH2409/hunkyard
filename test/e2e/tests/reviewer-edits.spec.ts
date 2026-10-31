import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { clearAllComments, publishFixtureReview, sessionCwd } from './helpers';

test('a reviewer edit on disk is published as an edit comment', async ({ request }) => {
  await clearAllComments(request);
  const file = path.join(await sessionCwd(request), 'server.go');
  const original = fs.readFileSync(file, 'utf8');
  const lines = original.split('\n');
  lines[0] = lines[0] + ' // reviewer edit';
  fs.writeFileSync(file, lines.join('\n'));
  try {
    const response = await publishFixtureReview(request);
    await expect(response).toBeOK();
    const { batch } = await response.json();
    const edit = batch.comments.find((c: { scope: string }) => c.scope === 'edit');
    expect(edit).toMatchObject({ id: 'edit:server.go:1', path: 'server.go', start_line: 1, quote: original.split('\n')[0] });
    expect(edit.body).toBe('Reviewer edited these lines:\n' + lines[0]);
  } finally {
    fs.writeFileSync(file, original);
  }
});
