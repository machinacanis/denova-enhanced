import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StoryStage } from './StoryStage'
import { sendInteractiveMessage } from '../api'
import type { InteractiveSSEEvent } from '../types'

vi.mock('../api', () => ({
  sendInteractiveMessage: vi.fn(),
}))

function streamEvents(events: InteractiveSSEEvent[]): ReadableStream<InteractiveSSEEvent> {
  return new ReadableStream<InteractiveSSEEvent>({
    start(controller) {
      for (const event of events) controller.enqueue(event)
      controller.close()
    },
  })
}

describe('StoryStage', () => {
  it('uses chat messages for interactive history and streamed agent events', async () => {
    vi.mocked(sendInteractiveMessage).mockResolvedValue(streamEvents([
      { event: 'thinking', data: JSON.stringify({ content: '先判断现场风险。' }) },
      { event: 'chunk', data: JSON.stringify({ content: '火光照亮了' }) },
      { event: 'chunk', data: JSON.stringify({ content: '墙上的新线索。' }) },
      { event: 'done', data: '{}' },
    ]))
    const onDone = vi.fn()

    render(
      <StoryStage
        storyId="st_1"
        branchId="main"
        snapshot={{
          story_id: 'st_1',
          branch_id: 'main',
          state: {},
          turns: [
            {
              id: 'ev_1',
              parent_id: null,
              branch_id: 'main',
              ts: '',
              user: '我推开酒馆的门',
              narrative: '门后传来低沉的风声。',
            },
          ],
        }}
        onDone={onDone}
      />,
    )

    expect(screen.getAllByText('Nova').length).toBeGreaterThan(0)
    expect(screen.getByText('我推开酒馆的门')).toBeInTheDocument()
    expect(screen.getByText('门后传来低沉的风声。')).toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('你要做什么？'), { target: { value: '我点燃火把' } })
    fireEvent.click(screen.getByRole('button', { name: /发送/ }))

    await screen.findByText('我点燃火把')
    await screen.findByText('先判断现场风险。')
    await screen.findByText(/火光照亮了墙上的新线索。/)
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
  })
})
