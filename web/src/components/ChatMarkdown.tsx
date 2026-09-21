import { memo } from 'react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

const plugins = [remarkGfm]

export const ChatMarkdown = memo(function ChatMarkdown({ children }: { children: string }) {
  return (
    <div className="chat-markdown">
      <Markdown remarkPlugins={plugins} skipHtml components={{
        a: ({ href, children }) => <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>,
        table: ({ children }) => <div className="overflow-x-auto"><table>{children}</table></div>,
      }}>
        {children}
      </Markdown>
    </div>
  )
})
