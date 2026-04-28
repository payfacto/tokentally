const activeTimers = new WeakMap<HTMLElement, ReturnType<typeof setTimeout>>()

export async function copyMarkdown(text: string, btn: HTMLElement): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    return
  }
  const prev = activeTimers.get(btn)
  if (prev !== undefined) clearTimeout(prev)
  btn.style.color = 'var(--good)'
  btn.style.borderColor = 'var(--good)'
  const timer = setTimeout(() => {
    btn.style.color = ''
    btn.style.borderColor = ''
    activeTimers.delete(btn)
  }, 1200)
  activeTimers.set(btn, timer)
}
