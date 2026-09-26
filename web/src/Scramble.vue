<script setup lang="ts">
// A line of text whose tail settles out of noise.
//
// The last few characters cycle through glyphs and land one at a time, left to
// right, so the line resolves rather than simply appearing. It reads as
// something being computed, which is what this page is about.
//
// The full text is always in the DOM as far as assistive technology is
// concerned — the scrambling characters are marked aria-hidden and the real
// string is exposed once — so nothing here is read out as noise. Anyone who
// asked for reduced motion gets the settled text immediately.
import { onBeforeUnmount, onMounted, ref } from "vue";

const props = withDefaults(
  defineProps<{
    text: string;
    /** How many trailing characters take part. */
    tail?: number;
    /** Delay before this line starts, so several can cascade. */
    delay?: number;
  }>(),
  { tail: 5, delay: 0 },
);

const GLYPHS = "#@*%$&/\\<>[]{}=+~^?012345789abcdefxyz";
const shown = ref(props.text);
let frame = 0;
let timer: ReturnType<typeof setTimeout> | undefined;

function run() {
  // Only the trailing characters move; whitespace and the full stop stay put so
  // the line keeps its shape while it settles.
  const chars = [...props.text];
  const movable: number[] = [];
  for (let i = chars.length - 1; i >= 0 && movable.length < props.tail; i--) {
    if (/\s/.test(chars[i]!)) continue;
    movable.unshift(i);
  }
  if (!movable.length) return;

  // Each character lands a beat after the one before it.
  const perChar = 8; // frames
  const total = movable.length * perChar + 10;
  let tick = 0;

  const step = () => {
    tick++;
    const out = [...chars];
    movable.forEach((index, order) => {
      const landsAt = (order + 1) * perChar;
      if (tick < landsAt) {
        out[index] = GLYPHS[Math.floor(Math.random() * GLYPHS.length)]!;
      }
    });
    shown.value = out.join("");
    if (tick < total) frame = requestAnimationFrame(step);
    else shown.value = props.text;
  };
  frame = requestAnimationFrame(step);
}

onMounted(() => {
  if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return;
  timer = setTimeout(run, props.delay);
});
onBeforeUnmount(() => {
  cancelAnimationFrame(frame);
  clearTimeout(timer);
});
</script>

<template><span class="scramble"><span aria-hidden="true">{{ shown }}</span><span class="sr-only">{{ text }}</span></span></template>
