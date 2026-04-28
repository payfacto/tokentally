declare const echarts: {
  init(el: HTMLElement, theme: null, opts: { renderer: string }): EChartsInstance
}

interface EChartsInstance {
  setOption(option: unknown): void
  resize(): void
}

const PALETTE = ['#eb733b', '#b04e20', '#2d8a5e', '#b07800', '#c03030', '#4ab0c0', '#9050b0']

const BASE = {
  textStyle: { color: '#1c1008', fontFamily: 'Figtree, system-ui, sans-serif' },
  color: PALETTE,
  grid: { left: 36, right: 12, top: 24, bottom: 24, containLabel: true },
}

const X_AXIS = {
  axisLine:  { lineStyle: { color: '#e2d0ba' } },
  axisLabel: { color: '#7a5c3a' },
  axisTick:  { show: false },
}

const Y_AXIS = {
  axisLine:  { show: false },
  axisTick:  { show: false },
  splitLine: { lineStyle: { color: '#e2d0ba' } },
  axisLabel: { color: '#7a5c3a' },
}

const TOOLTIP = {
  trigger: 'axis',
  backgroundColor: '#f8efe5',
  borderColor: '#cbb89e',
  borderWidth: 1,
  textStyle: { color: '#1c1008', fontFamily: 'Figtree, system-ui, sans-serif', fontSize: 12 },
  padding: [8, 12],
}

function mount(el: HTMLElement): EChartsInstance {
  const c = echarts.init(el, null, { renderer: 'svg' })
  window.addEventListener('resize', () => c.resize())
  return c
}

export interface BarChartOptions {
  categories: string[]
  values: number[]
  color?: string
}

export interface SeriesItem {
  name: string
  values: number[]
  color?: string
}

export interface StackedBarOptions {
  categories: string[]
  series: SeriesItem[]
  formatter?: (v: number) => string
}

export interface GroupedBarOptions {
  categories: string[]
  series: SeriesItem[]
  formatter?: (v: number) => string
}

export interface DonutDataItem {
  name: string
  value: number
}

export function barChart(el: HTMLElement, opts: BarChartOptions): void {
  const c = mount(el)
  c.setOption({
    ...BASE,
    tooltip: { ...TOOLTIP, axisPointer: { type: 'shadow' } },
    xAxis: {
      ...X_AXIS, type: 'category', data: opts.categories,
      axisLabel: { ...X_AXIS.axisLabel, interval: 0, rotate: opts.categories.length > 5 ? 25 : 0 },
    },
    yAxis: { ...Y_AXIS, type: 'value' },
    series: [{
      type: 'bar', data: opts.values,
      itemStyle: { color: opts.color || PALETTE[0], borderRadius: [4, 4, 0, 0] },
      barMaxWidth: 32,
    }],
  })
}

export function stackedBarChart(el: HTMLElement, opts: StackedBarOptions): void {
  const c = mount(el)
  c.setOption({
    ...BASE,
    tooltip: {
      ...TOOLTIP,
      axisPointer: { type: 'shadow' },
      valueFormatter: opts.formatter || ((v: number) => Number(v).toLocaleString()),
    },
    legend: { textStyle: { color: '#7a5c3a' }, top: 0, right: 0, icon: 'roundRect', itemWidth: 8, itemHeight: 8 },
    xAxis: {
      ...X_AXIS, type: 'category', data: opts.categories,
      axisLabel: {
        ...X_AXIS.axisLabel, interval: 'auto', rotate: 45,
        formatter: (val: string) => {
          const p = (val || '').split('-')
          if (p.length === 3) {
            const mo = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec']
            return mo[+p[1] - 1] + ' ' + +p[2]
          }
          return val
        },
      },
    },
    yAxis: { ...Y_AXIS, type: 'value' },
    series: opts.series.map((s, i) => ({
      name: s.name, type: 'bar', stack: 'total', data: s.values,
      itemStyle: { color: s.color || PALETTE[i % PALETTE.length] },
      barMaxWidth: 24, emphasis: { focus: 'series' },
    })),
  })
}

export function groupedBarChart(el: HTMLElement, opts: GroupedBarOptions): void {
  const c = mount(el)
  c.setOption({
    ...BASE,
    tooltip: {
      ...TOOLTIP,
      axisPointer: { type: 'shadow' },
      valueFormatter: opts.formatter || ((v: number) => Number(v).toLocaleString()),
    },
    legend: { textStyle: { color: '#7a5c3a' }, top: 0, right: 0, icon: 'roundRect', itemWidth: 8, itemHeight: 8 },
    xAxis: {
      ...X_AXIS, type: 'category', data: opts.categories,
      axisLabel: { ...X_AXIS.axisLabel, interval: 0, rotate: opts.categories.length > 5 ? 25 : 0 },
    },
    yAxis: { ...Y_AXIS, type: 'value' },
    series: opts.series.map((s, i) => ({
      name: s.name, type: 'bar', data: s.values,
      itemStyle: { color: s.color || PALETTE[i % PALETTE.length], borderRadius: [4, 4, 0, 0] },
      barMaxWidth: 24, emphasis: { focus: 'series' },
    })),
  })
}

export function donutChart(el: HTMLElement, data: DonutDataItem[]): void {
  const c = mount(el)
  c.setOption({
    color: PALETTE,
    tooltip: {
      trigger: 'item',
      backgroundColor: '#f8efe5', borderColor: '#cbb89e', borderWidth: 1,
      textStyle: { color: '#1c1008', fontFamily: 'Figtree, system-ui, sans-serif' },
      formatter: (p: { name: string; value: number; percent: number }) =>
        `${p.name}<br/><b>${Number(p.value).toLocaleString()}</b> tokens (${p.percent.toFixed(1)}%)`,
    },
    legend: {
      textStyle: { color: '#7a5c3a' }, bottom: 10, icon: 'roundRect',
      itemWidth: 8, itemHeight: 8, type: 'scroll',
    },
    series: [{
      type: 'pie', center: ['50%', '44%'], radius: ['48%', '68%'],
      avoidLabelOverlap: true, padAngle: 2,
      itemStyle: { borderColor: '#f8efe5', borderWidth: 2, borderRadius: 4 },
      label: {
        show: true, position: 'inside', color: '#fff', fontSize: 12, fontWeight: 600,
        formatter: ({ percent }: { percent: number }) => percent >= 6 ? percent.toFixed(0) + '%' : '',
      },
      labelLine: { show: false },
      data,
    }],
  })
}
