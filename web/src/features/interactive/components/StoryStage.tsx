import { useMemo, useState } from 'react'
import { Send } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { MessageList } from '@/components/Chat/MessageList'
import type { ChatMessage } from '@/lib/api'
import { sendInteractiveMessage } from '../api'
import type { Snapshot } from '../types'

interface StoryStageProps {
  storyId: string
  branchId: string
  snapshot: Snapshot | null
  onDone: () => void
}

export function StoryStage({ storyId, branchId, snapshot, onDone }: StoryStageProps) {
  const [input, setInput] = useState('')
  const [streaming, setStreaming] = useState(false)
  const [activityContent, setActivityContent] = useState('')
  const [liveMessages, setLiveMessages] = useState<ChatMessage[]>([])

  const historyMessages = useMemo<ChatMessage[]>(() => {
    return (snapshot?.turns || []).flatMap((turn) => [
      { id: `${turn.id}-user`, role: 'user' as const, content: turn.user },
      { id: `${turn.id}-assistant`, role: 'assistant' as const, content: turn.narrative },
    ])
  }, [snapshot?.turns])

  const visibleLiveMessages = useMemo(() => {
    if (streaming || liveMessages.length === 0) return liveMessages
    const lastTurn = snapshot?.turns?.[snapshot.turns.length - 1]
    const liveUser = liveMessages.find((msg) => msg.role === 'user')?.content || ''
    const liveAssistant = liveMessages
      .filter((msg) => msg.role === 'assistant')
      .map((msg) => msg.content || '')
      .join('')
    if (lastTurn && lastTurn.user === liveUser && lastTurn.narrative === liveAssistant) return []
    return liveMessages
  }, [liveMessages, snapshot?.turns, streaming])

  const messages = useMemo(() => [...historyMessages, ...visibleLiveMessages], [historyMessages, visibleLiveMessages])

  const send = async () => {
    const message = input.trim()
    if (!message || !storyId || streaming) return
    setInput('')
    setActivityContent('正在连接 AI Agent…')
    setLiveMessages([{ role: 'user', content: message }])
    setStreaming(true)
    try {
      const stream = await sendInteractiveMessage({ mode: 'story', story_id: storyId, branch: branchId, message })
      const reader = stream.getReader()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        switch (value.event) {
          case 'chunk': {
            const data = JSON.parse(value.data)
            appendAssistantMessage(data.content || '')
            setActivityContent('')
            break
          }
          case 'thinking': {
            const data = JSON.parse(value.data)
            appendThinkingMessage(data.content || '')
            setActivityContent('正在思考…')
            break
          }
          case 'tool_call': {
            const data = JSON.parse(value.data)
            setActivityContent('')
            setLiveMessages((prev) => [...prev, {
              id: data.id,
              role: 'tool_call',
              content: `调用工具 ${data.name || 'unknown_tool'}`,
              name: data.name || 'unknown_tool',
              args: data.args || '',
              status: 'running',
            }])
            break
          }
          case 'tool_args_delta': {
            const data = JSON.parse(value.data)
            setLiveMessages((prev) => prev.map((msg) => (
              msg.role === 'tool_call' && msg.id === data.id
                ? { ...msg, args: `${msg.args || ''}${data.delta || ''}` }
                : msg
            )))
            break
          }
          case 'tool_result': {
            const data = JSON.parse(value.data)
            setActivityContent('')
            setLiveMessages((prev) => prev.map((msg) => (
              msg.role === 'tool_call' && msg.id === data.id
                ? { ...msg, status: 'success', result: data.content || '' }
                : msg
            )))
            break
          }
          case 'error': {
            const data = JSON.parse(value.data)
            setActivityContent('')
            setLiveMessages((prev) => [...prev, { role: 'error', content: data.message || data.error || '未知错误' }])
            break
          }
          case 'done': {
            setActivityContent('完成')
            break
          }
        }
      }
      await onDone()
    } finally {
      setStreaming(false)
      setActivityContent('')
    }
  }

  return (
    <main className="flex min-w-0 flex-1 flex-col bg-[#18191b] p-3">
      <div data-testid="story-stage-card" className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-[#333842] bg-[#141519]">
        <div className="flex h-10 items-center justify-between px-4">
          <div className="text-xs font-medium text-[#7f8898]">故事舞台 · 当前分支 {branchId || 'main'}</div>
          <Badge variant="outline" className="border-[#333842] bg-[#20242b] text-[#7f8898]">{snapshot?.turns?.length || 0} 回合</Badge>
        </div>
        {messages.length === 0 && !streaming ? (
          <div className="flex min-h-0 flex-1 items-center justify-center rounded-xl border border-dashed border-[#333842] bg-[#18191b]/80 text-sm text-[#858b96]">
            输入第一句话，开始互动故事
          </div>
        ) : (
          <MessageList messages={messages} isStreaming={streaming} activityContent={activityContent} />
        )}
      </div>
      <div className="mt-3 rounded-xl border border-[#333842] bg-[#141519] p-3">
        <div className="flex items-center gap-3">
          <Textarea
            className="h-14 min-h-14 flex-1 resize-none border-[#333842] bg-[#1f2228] text-sm text-[#d7dbe2] placeholder:text-[#778091] focus-visible:ring-1"
            value={input}
            placeholder="你要做什么？"
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) void send()
            }}
          />
          <Button className="h-14 w-24" disabled={!storyId || streaming || !input.trim()} onClick={() => void send()}>
            <Send className="h-4 w-4" />
            {streaming ? '生成中' : '发送'}
          </Button>
        </div>
      </div>
    </main>
  )

  function appendAssistantMessage(content: string) {
    if (!content) return
    setLiveMessages((prev) => {
      const last = prev[prev.length - 1]
      if (last?.role === 'assistant' && last.streaming) {
        return [...prev.slice(0, -1), { ...last, content: `${last.content || ''}${content}` }]
      }
      return [...prev, { role: 'assistant', content, streaming: true }]
    })
  }

  function appendThinkingMessage(content: string) {
    if (!content) return
    setLiveMessages((prev) => {
      const last = prev[prev.length - 1]
      if (last?.role === 'thinking' && last.streaming) {
        return [...prev.slice(0, -1), { ...last, content: `${last.content || ''}${content}` }]
      }
      return [...prev, { role: 'thinking', content, streaming: true }]
    })
  }
}
