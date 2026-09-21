import { memo, useRef, useState, type ReactNode } from 'react'
import { Check, Copy } from 'lucide-react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

const plugins = [remarkGfm]

function CodeBlock({ children }: { children?: ReactNode }) {
  const code = useRef<HTMLPreElement>(null)
  const [status, setStatus] = useState<'idle' | 'copied' | 'failed'>('idle')
  return <div className="my-3 overflow-hidden rounded-lg border bg-background">
    <div className="flex items-center justify-between border-b bg-muted/40 px-3 py-1.5 text-[11px] text-muted-foreground">
      <span className="font-mono">Code</span>
      <button className="inline-flex items-center gap-1.5 rounded px-1.5 py-1 hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring" onClick={async () => {
        try { await navigator.clipboard.writeText(code.current?.textContent ?? ''); setStatus('copied') } catch { setStatus('failed') }
      }}>{status === 'copied' ? <Check size={12} /> : <Copy size={12} />}<span role="status">{status === 'copied' ? 'Copied' : status === 'failed' ? 'Retry copy' : 'Copy'}</span></button>
    </div>
    <pre ref={code} style={{ margin: 0, border: 0, borderRadius: 0 }}>{children}</pre>
  </div>
}

export const ChatMarkdown = memo(function ChatMarkdown({ children }: { children: string }) {
  return (
    <div className="chat-markdown">
      <Markdown remarkPlugins={plugins} skipHtml components={{
        a: ({ href, children }) => <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>,
        table: ({ children }) => <div className="overflow-x-auto"><table>{children}</table></div>,
        pre: ({ children }) => <CodeBlock>{children}</CodeBlock>,
      }}>
        {children}
      </Markdown>
    </div>
  )
})
