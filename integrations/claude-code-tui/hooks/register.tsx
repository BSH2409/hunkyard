import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register, Timer } from 'claude-code'

import type { ChatIndex, Doc, Source, Thread, UiState } from '../types'
import {
  EMPTY_UI,
  TOOL_PREFIX,
  chatDoc,
  claudeIdsOf,
  clientPropsOf,
  hunkyardThreadsOf,
  emptyDoc,
  feedbackOf,
  fileDoc,
  newId,
  parseDiff,
  pendingOf,
  quoteOffsetOf,
  chatLinePrefix,
  chatMarkdownOf,
  chatPlanSlug,
  chatThreadsFromPlan,
  sectionBefore,
  selectionOf,
  whereOf,
} from './model'

type $ = EngineInterface

export const PANE = 'hunkyard'
const MAX_UNTRACKED = 20

const threadsA = atom({ plugin: 'hunkyard-tui', key: 'threads' } as const, [] as Thread[])
const hunkyardThreadsA = atom({ plugin: 'hunkyard-tui', key: 'hunkyardThreads' } as const, [] as Thread[])
const sentA = atom({ plugin: 'hunkyard-tui', key: 'sent' } as const, [] as string[])
const seenA = atom({ plugin: 'hunkyard-tui', key: 'seen' } as const, [] as string[])
const uiA = atom({ plugin: 'hunkyard-tui', key: 'ui' } as const, EMPTY_UI)
const chatIndexA = atom({ plugin: 'hunkyard-tui', key: 'chatIndex' } as const, [] as ChatIndex)
const isOpenA = atom({ plugin: 'hunkyard-tui', key: 'isOpen' } as const, false)
const docsA = atom({ plugin: 'hunkyard-tui', key: 'docs' } as const, {
  chat: emptyDoc('chat', 'Conversation', 'Loading…'),
  diff: emptyDoc('diff', 'Diff', 'Loading…'),
  file: emptyDoc('file', 'File', 'Open a file with /hunkyard-tui file <path>.'),
})

// Values a reload recomputes; the review itself lives in $.state.
let hunkyardBin = 'hunkyard'
let reviewFile: string | null = null
let hasHunkyard = true
let diffTimer: Timer | null = null
// Chat comments live in a hunkyard plan when the hunkyard CLI has `plan --no-wait`.
let hasChatPlans: boolean | null = null
let chatSlug: string | null = null
let chatPlanFile: string | null = null
let savedChat = ''

// ── Loading documents ─────────────────────────────────────────────────

async function setDoc($: $, source: Source, doc: Doc) {
  await update($, docsA, docs => ({ ...docs, [source]: doc }))
}

async function refreshChat($: $) {
  const messages = await $.session.messages().catch(() => [])
  const { doc, index } = chatDoc(messages)
  await setDoc($, 'chat', doc)
  await update($, chatIndexA, () => index)
  // Once the chat has comments in hunkyard, each turn is a new plan version.
  if (savedChat !== '') {
    await saveChatPlan($)
  }
}

async function git($: $, argv: string[]) {
  return $.process.run(['git', ...argv], { timeoutMs: 15000 })
}

async function refreshDiff($: $) {
  const inside = await git($, ['rev-parse', '--is-inside-work-tree']).catch(() => null)
  if (!inside || inside.exitCode !== 0) {
    await setDoc($, 'diff', emptyDoc('diff', 'Diff', 'Not a git repository.'))
    return
  }

  let tracked = await git($, ['diff', 'HEAD', '--no-color', '--no-ext-diff', '-U3'])
  if (tracked.exitCode !== 0) {
    tracked = await git($, ['diff', '--no-color', '--no-ext-diff', '-U3'])
  }

  const others = await git($, ['ls-files', '--others', '--exclude-standard'])
  const untracked = others.stdout.split('\n').filter(Boolean).slice(0, MAX_UNTRACKED)
  const added = await Promise.all(
    untracked.map(path =>
      git($, ['diff', '--no-color', '--no-index', '--', '/dev/null', path])
        .then(run => run.stdout)
        .catch(() => ''),
    ),
  )

  const rows = parseDiff([tracked.stdout, ...added].join('\n'))
  const files = rows.filter(row => row.kind === 'title').length
  await setDoc(
    $,
    'diff',
    rows.length === 0
      ? emptyDoc('diff', 'Diff', 'No uncommitted changes.')
      : { source: 'diff', title: `${files} file${files === 1 ? '' : 's'} changed`, rows },
  )
}

async function refreshFile($: $, path: string) {
  try {
    await setDoc($, 'file', fileDoc(path, await $.fs.read(path)))
  } catch (error) {
    await setDoc($, 'file', emptyDoc('file', path, `Cannot read ${path}: ${String(error)}`))
  }
}

async function refreshSource($: $, source: Source) {
  if (source === 'chat') {
    await refreshChat($)
  } else if (source === 'diff') {
    await refreshDiff($)
  } else {
    const { filePath } = await readUi($)
    if (filePath) {
      await refreshFile($, filePath)
    }
  }
}

// ── hunkyard's review file ────────────────────────────────────────────────

async function hunkyard($: $, argv: string[], stdin?: string) {
  const run = await $.process.run([hunkyardBin, ...argv], { timeoutMs: 15000, ...(stdin ? { stdin } : {}) })
  if (run.exitCode !== 0) {
    throw new Error((run.stderr || run.stdout).trim() || `hunkyard ${argv[0]} failed`)
  }
  return run.stdout
}

async function refreshHunkyard($: $) {
  if (!hasHunkyard) {
    return
  }
  try {
    if (!reviewFile) {
      const status = JSON.parse(await hunkyard($, ['status', '--json'])) as { review_file?: string }
      reviewFile = status.review_file ?? null
    }
    const code = reviewFile ? hunkyardThreadsOf(await $.fs.read(reviewFile).catch(() => '')) : []
    const chat = await readChatPlanThreads($)
    await update($, hunkyardThreadsA, () => [...code, ...chat])
  } catch {
    hasHunkyard = false
  }
}

// ── The conversation as a hunkyard plan ─────────────────────────────────────

async function chatPlanReady($: $): Promise<boolean> {
  if (!hasHunkyard) {
    return false
  }
  if (hasChatPlans === null) {
    const help = await $.process.run([hunkyardBin, 'plan', '--help'], { timeoutMs: 15000 }).catch(() => null)
    hasChatPlans = help !== null && `${help.stdout}${help.stderr}`.includes('--no-wait')
  }
  if (hasChatPlans && !chatSlug) {
    chatSlug = chatPlanSlug(await $.session.id())
    const home = await $.env.get('HOME')
    chatPlanFile = home ? `${home}/.hunkyard/plans/${chatSlug}/.hunkyard/review.json` : null
  }
  return hasChatPlans
}

/** Saves the conversation as the plan's next version (hunkyard adds none when unchanged). */
async function saveChatPlan($: $) {
  if (!(await chatPlanReady($)) || !chatSlug) {
    return
  }
  const markdown = chatMarkdownOf((await read($, docsA)).chat)
  if (markdown === savedChat) {
    return
  }
  await hunkyard($, ['plan', '--name', chatSlug, '--no-wait', '--quiet'], markdown)
  savedChat = markdown
}

async function readChatPlanThreads($: $): Promise<Thread[]> {
  if (!(await chatPlanReady($)) || !chatPlanFile) {
    return []
  }
  const json = await $.fs.read(chatPlanFile).catch(() => '')
  if (json === '') {
    return []
  }
  // A plan from earlier in this session: keep saving it each turn.
  if (savedChat === '') {
    savedChat = 'saved'
  }
  return chatThreadsFromPlan(hunkyardThreadsOf(json), (await read($, docsA)).chat.rows)
}

/** `hunkyard comment` arguments that point at the chat plan for a chat thread. */
function planArgs(thread: Pick<Thread, 'kind'>): string[] {
  return thread.kind === 'chat' && chatSlug ? ['--plan', chatSlug] : []
}

// ── Review state ──────────────────────────────────────────────────────

async function allThreads($: $) {
  return [...(await read($, threadsA)), ...(await read($, hunkyardThreadsA))]
}

function threadsFor(source: Source, local: readonly Thread[], fromHunkyard: readonly Thread[]) {
  const kind = source === 'chat' ? 'chat' : 'code'
  return [...fromHunkyard, ...local].filter(thread => thread.kind === kind)
}

async function sync($: $) {
  const threads = await allThreads($)
  const pending = pendingOf(threads, await read($, sentA), await read($, seenA))
  const drafts = pending.comments.length + pending.replies.length
  const parts = [drafts ? `${drafts} to send` : '', pending.unread ? `${pending.unread} new from Claude` : '']
  const text = parts.filter(Boolean).join(' · ')
  $.ui.status(text ? `hunkyard: ${text}` : undefined)
}

async function markSeen($: $) {
  const ids = claudeIdsOf(await allThreads($))
  await update($, seenA, seen => [...new Set([...seen, ...ids])])
}

async function findThread($: $, id: string): Promise<Thread | undefined> {
  const threads = await allThreads($)
  return threads.find(thread => thread.id === id) ?? threads.find(thread => id.length >= 4 && thread.id.startsWith(id))
}

async function addReply($: $, thread: Thread, body: string, who: 'you' | 'claude', resolve: boolean) {
  if ((await read($, hunkyardThreadsA)).some(one => one.id === thread.id)) {
    const argv = ['comment', ...planArgs(thread), '--reply-to', thread.id, ...(who === 'claude' ? ['--author', 'Claude'] : []), ...(resolve ? ['--resolve'] : []), body]
    await hunkyard($, argv)
    await refreshHunkyard($)
    return
  }
  await update($, threadsA, list =>
    list.map(one =>
      one.id === thread.id
        ? {
            ...one,
            resolved: resolve ? true : one.resolved,
            replies: [
              ...one.replies,
              { id: newId(), author: who, body, at: new Date().toISOString(), fromClaude: who === 'claude' },
            ],
          }
        : one,
    ),
  )
}

// ── Pane actions, mostly asked for by the Client ──────────────────────

/** The pane's settings, filled in from the defaults (an older build stored fewer fields). */
async function readUi($: $): Promise<UiState> {
  return { ...EMPTY_UI, ...(await read($, uiA)) }
}

async function setUi($: $, fn: (ui: UiState) => UiState) {
  await update($, uiA, now => fn({ ...EMPTY_UI, ...now }))
}

async function openSource($: $, source: Source) {
  const ui = await readUi($)
  if (source === 'file' && !ui.filePath) {
    $.ui.toast('Open a file first: /hunkyard-tui file <path>')
    return
  }
  await refreshSource($, source)
  const doc = (await read($, docsA))[source]
  const startAt = source === 'chat' ? sectionBefore(doc.rows, doc.rows.length) : 0
  await setUi($, now => ({ ...now, source, startAt, center: startAt, docRev: now.docRev + 1 }))
}

async function openPane($: $, source?: Source) {
  const ui = await readUi($)
  if (source || ui.docRev === 0) {
    await openSource($, source ?? ui.source)
  } else {
    await refreshSource($, ui.source)
  }
  await refreshHunkyard($)
  await update($, isOpenA, () => true)
  // Inline above the prompt a pane opens a third of the screen tall; ask for more.
  await $.ui.open({ id: PANE, title: 'hunkyard', focus: true, rows: 28 })
  await markSeen($)
  await sync($)
}

async function addComment($: $, start: number, end: number, body: string, cols?: { startCol: number | null; endCol: number | null }) {
  const ui = await readUi($)
  const picked = selectionOf((await read($, docsA))[ui.source], start, end, cols)
  if (typeof picked === 'string') {
    $.ui.toast(picked)
    return
  }
  const isChat = picked.path.startsWith('chat:')
  const doc = (await read($, docsA))[ui.source]
  if (isChat && (await chatPlanReady($)) && chatSlug) {
    // Chat comments go to the plan: its line numbers are row indices plus one.
    await saveChatPlan($)
    const lo = Math.min(start, end)
    const hi = Math.max(start, end)
    const lines = doc.rows.slice(lo, hi + 1).map(row => `${chatLinePrefix(row)}${row.text}`)
    const firstPrefix = doc.rows[lo] ? chatLinePrefix(doc.rows[lo]) : ''
    const offset = picked.isExact && cols?.startCol != null ? quoteOffsetOf(lines, cols.startCol + firstPrefix.length, picked.quote) : undefined
    const entry = {
      file: `${chatSlug}.md`,
      line: lo === hi ? lo + 1 : `${lo + 1}-${hi + 1}`,
      body,
      ...(picked.isExact && picked.quote ? { quote: picked.quote } : {}),
      ...(offset !== undefined ? { quote_offset: offset } : {}),
    }
    await hunkyard($, ['comment', '--plan', chatSlug, '--json'], JSON.stringify([entry]))
    await refreshHunkyard($)
    return
  }
  if (!isChat && hasHunkyard) {
    const line = picked.start === picked.end ? picked.start : `${picked.start}-${picked.end}`
    // An exact selection carries hunkyard's quote fields, as a web UI selection does.
    const lines = doc.rows
      .filter(row => row.path === picked.path && row.line !== undefined && row.line >= picked.start && row.line <= picked.end && row.sign !== '-')
      .map(row => row.text)
    const offset = picked.isExact && cols?.startCol != null ? quoteOffsetOf(lines, cols.startCol, picked.quote) : undefined
    const entry = {
      file: picked.path,
      line,
      body,
      ...(picked.isExact && picked.quote ? { quote: picked.quote } : {}),
      ...(offset !== undefined ? { quote_offset: offset } : {}),
    }
    await hunkyard($, ['comment', '--json'], JSON.stringify([entry]))
    await refreshHunkyard($)
    return
  }
  const { isExact: _isExact, ...anchor } = picked
  const thread: Thread = {
    id: newId(),
    kind: isChat ? 'chat' : 'code',
    ...anchor,
    body,
    author: 'you',
    fromClaude: false,
    at: new Date().toISOString(),
    resolved: false,
    replies: [],
  }
  await update($, threadsA, list => [...list, thread])
}

async function replyAsYou($: $, id: string, body: string) {
  const thread = await findThread($, id)
  if (thread) {
    await addReply($, thread, body, 'you', false)
  }
}

async function toggleResolve($: $, id: string) {
  const target = await findThread($, id)
  if (!target) {
    return
  }
  const isLocal = (await read($, threadsA)).some(one => one.id === target.id)
  if (isLocal) {
    await update($, threadsA, list => list.map(one => (one.id === target.id ? { ...one, resolved: !one.resolved } : one)))
  } else if (!target.resolved) {
    await hunkyard($, ['comment', ...planArgs(target), '--reply-to', target.id, '--resolve', 'Resolved.'])
    await refreshHunkyard($)
  } else {
    $.ui.toast('Reopen this one in the hunkyard browser view.')
  }
}

type ClientMessage = {
  type?: string
  center?: number
  start?: number
  end?: number
  startCol?: number | null
  endCol?: number | null
  body?: string
  id?: string
  source?: string
}

const isSource = (value: unknown): value is Source => value === 'chat' || value === 'diff' || value === 'file'

async function onClientMessage($: $, data: unknown) {
  const message = (data ?? {}) as ClientMessage
  try {
    switch (message.type) {
      case 'window':
        if (typeof message.center === 'number') {
          const center = message.center
          await setUi($, now => ({ ...now, center }))
        }
        return
      case 'comment':
        if (typeof message.start === 'number' && typeof message.end === 'number' && message.body) {
          const cols = typeof message.startCol === 'number' && typeof message.endCol === 'number' ? { startCol: message.startCol, endCol: message.endCol } : undefined
          await addComment($, message.start, message.end, message.body, cols)
        }
        break
      case 'reply':
        if (message.id && message.body) {
          await replyAsYou($, message.id, message.body)
        }
        break
      case 'resolve':
        if (message.id) {
          await toggleResolve($, message.id)
        }
        break
      case 'send':
        await send($)
        break
      case 'ask':
        await askForReview($)
        break
      case 'seen':
        break
      case 'toggleResolved':
        await setUi($, now => ({ ...now, resolvedView: now.resolvedView === 'line' ? 'hidden' : now.resolvedView === 'hidden' ? 'full' : 'line' }))
        break
      case 'refresh':
        await refreshSource($, (await readUi($)).source)
        await refreshHunkyard($)
        break
      case 'source':
        if (isSource(message.source)) {
          await openSource($, message.source)
        }
        break
    }
  } catch (error) {
    $.ui.toast(`hunkyard: ${String(error).slice(0, 160)}`)
  }
  await markSeen($)
  await sync($)
}

async function send($: $) {
  const threads = await allThreads($)
  const pending = pendingOf(threads, await read($, sentA), await read($, seenA))
  if (pending.comments.length + pending.replies.length === 0) {
    $.ui.toast('Nothing new to send.')
    return
  }
  const text = feedbackOf(pending, await read($, chatIndexA))
  const ids = [...pending.comments.map(one => one.id), ...pending.replies.map(one => one.reply.id)]
  await update($, sentA, sent => [...new Set([...sent, ...ids])])
  await $.prompt.submit({ text, asUser: true })
  $.ui.toast(`Sent ${ids.length} item${ids.length === 1 ? '' : 's'} to Claude.`)
  await sync($)
}

async function askForReview($: $) {
  await $.prompt.submit({
    text: [
      'Review the uncommitted changes (git diff HEAD, plus new files).',
      `Leave each finding as an inline comment with the ${TOOL_PREFIX}comment tool: file, line (new-side line number), optional end_line, and a short body.`,
      'Do not change any code yet. I will reply to your comments in the hunkyard pane.',
    ].join(' '),
    asUser: true,
  })
  $.ui.toast('Asked Claude to review the diff.')
}

export const register: Register = (on, options) => {
  hunkyardBin = typeof options.hunkyardBin === 'string' && options.hunkyardBin !== '' ? options.hunkyardBin : 'hunkyard'
  reviewFile = null
  hasHunkyard = true

  // ── Session ───────────────────────────────────────────────────────────

  on('session.start', async ($, e, next) => {
    const started = await next(e)

    await $.command.register({
      name: 'hunkyard-tui',
      description: 'hunkyard: review the chat, your diff or a file, in a pane',
      argumentHint: '[chat|diff|file <path>|send|close]',
      immediate: true,
    })

    await $.tool.register({
      name: 'reply',
      description:
        'Reply to a hunkyard review comment the user left (on the conversation, the diff or a file). Pass the comment id from the review and a short reply. The user resolves threads, not you.',
      inputSchema: {
        type: 'object',
        properties: {
          id: { type: 'string', description: 'The comment id, as given in the review.' },
          body: { type: 'string', description: 'Your reply.' },
        },
        required: ['id', 'body'],
      },
    })

    await $.tool.register({
      name: 'comment',
      description:
        'Leave an inline hunkyard review comment on a line of a file in the repository (new-side line numbers for diffs). The user reads it in the hunkyard pane and can reply.',
      inputSchema: {
        type: 'object',
        properties: {
          file: { type: 'string', description: 'Path relative to the repository root.' },
          line: { type: 'integer', minimum: 1 },
          end_line: { type: 'integer', minimum: 1 },
          body: { type: 'string' },
        },
        required: ['file', 'line', 'body'],
      },
    })

    await $.tool.register({
      name: 'threads',
      description: 'List open hunkyard review threads (comments and replies) with their ids.',
      inputSchema: { type: 'object', properties: { include_resolved: { type: 'boolean' } } },
    })

    await refreshHunkyard($)
    await sync($)

    return started
  })

  on('command.run', { command: 'hunkyard-tui' }, async ($, e) => {
    const [word = '', ...rest] = e.args.trim().split(/\s+/)
    const arg = rest.join(' ')

    switch (word) {
      case 'send':
        await send($)
        return { text: 'hunkyard: review sent.' }
      case 'close':
        await $.ui.close({ id: PANE })
        await update($, isOpenA, () => false)
        return { text: 'hunkyard pane closed.' }
      case 'file': {
        if (!arg) {
          return { text: 'Usage: /hunkyard-tui file <path>' }
        }
        await setUi($, now => ({ ...now, filePath: arg }))
        await openPane($, 'file')
        return { text: `hunkyard: reviewing ${arg}.` }
      }
      case 'chat':
      case 'diff':
        await openPane($, word)
        return { text: `hunkyard: reviewing the ${word === 'chat' ? 'conversation' : 'diff'}.` }
      case '':
        await openPane($)
        return { text: 'hunkyard pane open. Click it to give it the keys.' }
      default:
        return { text: `Unknown /hunkyard-tui argument "${word}". Try chat, diff, file <path>, send or close.` }
    }
  })

  on('ui.close', { id: PANE }, async ($, e, next) => {
    await update($, isOpenA, () => false)
    return next(e)
  })

  // ── Tools Claude calls ────────────────────────────────────────────────

  on('tool.call', { tool: 'mcp__hunkyard-tui__reply' }, async ($, e) => {
    const input = e as unknown as { id?: string; body?: string }
    const thread = input.id ? await findThread($, input.id) : undefined
    if (!thread || !input.body) {
      return { deny: `No hunkyard thread with id ${input.id ?? '(none)'}. Call ${TOOL_PREFIX}threads to list them.` }
    }
    try {
      await addReply($, thread, input.body, 'claude', false)
    } catch (error) {
      return { deny: `hunkyard could not save the reply: ${String(error)}` }
    }
    $.ui.toast(`Claude replied on ${whereOf(thread, await read($, chatIndexA))}`)
    await sync($)
    return { result: 'Reply saved. The user sees it in the hunkyard pane.' }
  })

  on('tool.call', { tool: 'mcp__hunkyard-tui__comment' }, async ($, e) => {
    const input = e as unknown as { file?: string; line?: number; end_line?: number; body?: string }
    if (!input.file || !input.line || !input.body) {
      return { deny: 'file, line and body are required.' }
    }
    const end = input.end_line && input.end_line > input.line ? input.end_line : input.line
    try {
      if (hasHunkyard) {
        await hunkyard($, ['comment', '--author', 'Claude', `${input.file}:${input.line}${end !== input.line ? `-${end}` : ''}`, input.body])
        await refreshHunkyard($)
      } else {
        const thread: Thread = {
          id: newId(),
          kind: 'code',
          path: input.file,
          start: input.line,
          end,
          quote: '',
          body: input.body,
          author: 'claude',
          fromClaude: true,
          at: new Date().toISOString(),
          resolved: false,
          replies: [],
        }
        await update($, threadsA, list => [...list, thread])
      }
    } catch (error) {
      return { deny: `hunkyard could not save the comment: ${String(error)}` }
    }
    await sync($)
    return { result: `Comment left on ${input.file}:${input.line}.` }
  })

  on('tool.call', { tool: 'mcp__hunkyard-tui__threads' }, async ($, e) => {
    const input = e as unknown as { include_resolved?: boolean }
    const index = await read($, chatIndexA)
    const threads = (await allThreads($)).filter(thread => input.include_resolved || !thread.resolved)
    if (threads.length === 0) {
      return { result: 'No open hunkyard threads.' }
    }
    const text = threads
      .map(thread =>
        [
          `[id ${thread.id}] ${whereOf(thread, index)}${thread.resolved ? ' (resolved)' : ''}`,
          `  ${thread.fromClaude ? 'claude' : 'user'}: ${thread.body}`,
          ...thread.replies.map(reply => `  ${reply.fromClaude ? 'claude' : 'user'}: ${reply.body}`),
        ].join('\n'),
      )
      .join('\n\n')
    return { result: text }
  })

  // ── Keeping documents fresh ───────────────────────────────────────────

  on('tool.call', { tool: ['Edit', 'Write', 'NotebookEdit'] }, async ($, e, next) => {
    const ran = await next(e)
    if ((await read($, isOpenA)) && (await readUi($)).source === 'diff') {
      diffTimer?.cancel()
      diffTimer = $.clock.after(500, () => {
        void refreshDiff($)
      })
    }
    return ran
  })

  on('turn.complete', async ($, e, next) => {
    const done = await next(e)
    await refreshChat($)
    await refreshHunkyard($)
    if ((await read($, isOpenA)) && (await readUi($)).source === 'diff') {
      await refreshDiff($)
    }
    await sync($)
    return done
  })

  // ── The Client's messages, and the wheel over the pane ────────────────

  on('ui.message', { requestId: PANE }, async ($, e) => {
    await onClientMessage($, e.data)
    return {}
  })

  on('ui.scroll', { requestId: PANE }, async ($, e, next) => {
    if (e.origin.kind !== 'person') {
      return next(e)
    }
    const by = e.by
    await setUi($, now => ({ ...now, wheel: now.wheel + 1, wheelBy: by }))
    return {}
  })

  // ── The pane: one Client that owns keys, mouse and drawing ────────────

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    if (e.surface === 'mobile' || e.surface === 'vscode') {
      const { Text } = $.ui.resolve(e)
      return <Text dimColor>hunkyard-tui needs the terminal or the desktop app.</Text>
    }

    const { Client } = $.ui.resolve(e)
    const ui = await readUi($)
    const local = await read($, threadsA)
    const fromHunkyard = await read($, hunkyardThreadsA)
    const sent = await read($, sentA)
    const seen = await read($, seenA)
    const pending = pendingOf([...local, ...fromHunkyard], sent, seen)
    const drafts = pending.comments.length + pending.replies.length
    const status = hasHunkyard ? '' : 'no hunkyard review file: code comments stay here'

    const props = clientPropsOf({
      doc: (await read($, docsA))[ui.source],
      threads: threadsFor(ui.source, local, fromHunkyard),
      ui,
      sent,
      seen,
      status,
      toSend: drafts,
      unread: pending.unread,
    })

    return (
      <Client
        key="review"
        module="./client.tsx"
        props={props}
        width={Math.max(30, e.props.bodyColumns)}
        height={Math.max(6, e.props.scroll.bodyRows)}
      />
    )
  })

  // ── The band above the prompt while the pane is closed ────────────────

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    if (e.props.hasSurvey || (await read($, isOpenA))) {
      return next(e)
    }
    const pending = pendingOf(await allThreads($), await read($, sentA), await read($, seenA))
    const drafts = pending.comments.length + pending.replies.length
    if (drafts === 0 && pending.unread === 0) {
      return next(e)
    }
    const { Box, Text, Button } = $.ui.resolve(e)
    const parts = [
      drafts ? `${drafts} comment${drafts === 1 ? '' : 's'} to send` : '',
      pending.unread ? `${pending.unread} new from Claude` : '',
    ]
    return (
      <Box flexDirection="row" gap={2}>
        <Text color="magenta">hunkyard</Text>
        <Text dimColor>{parts.filter(Boolean).join(' · ')}</Text>
        {drafts > 0 && <Button key="band-send" hotkey="s" label="send" variant="primary" onPress={() => void send($)} />}
        <Button key="band-open" hotkey="o" label="open" onPress={() => void openPane($)} />
      </Box>
    )
  })

  // ── A note under replies that have comments ───────────────────────────

  on('ui.render', { component: 'AssistantMessage' }, async ($, e, next) => {
    const text = e.props.text.trim()
    if (text === '') {
      return next(e)
    }
    const index = await read($, chatIndexA)
    const probe = text.slice(0, 200)
    const entry = index.find(one => one.role === 'assistant' && one.text.includes(probe))
    if (!entry) {
      return next(e)
    }
    const all = [...(await read($, threadsA)), ...(await read($, hunkyardThreadsA))]
    const mine = all.filter(thread => thread.path === `chat:${entry.index}`)
    if (mine.length === 0) {
      return next(e)
    }
    const open = mine.filter(thread => !thread.resolved).length
    const replies = mine.reduce((sum, thread) => sum + thread.replies.length, 0)
    const drawn = await next(e)
    const { Box, Text } = $.ui.resolve(e)
    return (
      <Box flexDirection="column">
        {drawn}
        <Text color="magenta" dimColor>
          {'  '}💬 {mine.length} hunkyard comment{mine.length === 1 ? '' : 's'} · {open} open
          {replies ? ` · ${replies} repl${replies === 1 ? 'y' : 'ies'}` : ''} · /hunkyard-tui to open
        </Text>
      </Box>
    )
  }).catch(($, e, next) => next(e))
}
