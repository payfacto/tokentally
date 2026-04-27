export async function copyMarkdown(text: string, btn: HTMLElement): Promise<void> {
  await navigator.clipboard.writeText(text)
  btn.style.color = 'var(--good)'
  btn.style.borderColor = 'var(--good)'
  setTimeout(() => { btn.style.color = ''; btn.style.borderColor = '' }, 1200)
}
