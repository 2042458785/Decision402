<script setup lang="ts">
// A select that the page draws itself.
//
// A native <select> renders its open list with the operating system, so the
// list ignores the page entirely — a different typeface, different colours,
// rounded corners the rest of the interface does not have. This replaces the
// list, and nothing else: the value still travels by v-model, and a change
// still emits once, so it drops in where the native element was.
//
// It behaves like the control it replaces: click or Enter or Space to open,
// arrows to move, Enter to take, Escape to abandon, and clicking away closes.
//
// Clicks are stopped and their default prevented because callers wrap this in
// a <label>, and a label re-dispatches clicks to its labelable control — a
// <button> is one. Without this, choosing an option closed the list and the
// label immediately reopened it by forwarding the same click to the face.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

// `short` is what the control says when closed, where the column is narrow.
// `label` is always what the open list says, in full.
type Option = { value: string | number; label: string; short?: string };

const props = defineProps<{
  modelValue: string | number;
  options: Option[];
  disabled?: boolean;
}>();
const emit = defineEmits<{
  (e: "update:modelValue", value: string | number): void;
  (e: "change"): void;
}>();

const open = ref(false);
const root = ref<HTMLElement | null>(null);
const active = ref(0);

const current = computed(
  () => props.options.find((o) => o.value === props.modelValue) ?? props.options[0],
);

function show() {
  if (props.disabled) return;
  active.value = Math.max(
    0,
    props.options.findIndex((o) => o.value === props.modelValue),
  );
  open.value = true;
}
function hide() {
  open.value = false;
}
function take(option: Option) {
  hide();
  if (option.value !== props.modelValue) {
    emit("update:modelValue", option.value);
    // The native element fires change after the value settles, and callers
    // read the new value in that handler, so this waits for the same moment.
    nextTick(() => emit("change"));
  }
}
function onKey(event: KeyboardEvent) {
  if (props.disabled) return;
  if (!open.value) {
    if (["Enter", " ", "ArrowDown", "ArrowUp"].includes(event.key)) {
      event.preventDefault();
      show();
    }
    return;
  }
  if (event.key === "Escape") {
    event.preventDefault();
    hide();
  } else if (event.key === "ArrowDown") {
    event.preventDefault();
    active.value = (active.value + 1) % props.options.length;
  } else if (event.key === "ArrowUp") {
    event.preventDefault();
    active.value = (active.value - 1 + props.options.length) % props.options.length;
  } else if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    const option = props.options[active.value];
    if (option) take(option);
  }
}
function onDocument(event: MouseEvent) {
  if (root.value && !root.value.contains(event.target as Node)) hide();
}
onMounted(() => document.addEventListener("mousedown", onDocument));
onBeforeUnmount(() => document.removeEventListener("mousedown", onDocument));
// An option can disappear while the list is open — the advisory risk level is
// only offered in simulation — so the list closes rather than pointing at a
// row that is no longer there.
//
// This watches what the options ARE, not the array they arrive in. Callers pass
// an inline literal, so the array is a new object on every render: watching its
// identity fired on every keystroke elsewhere on the page and shut the list
// the moment it opened.
const signature = computed(() => props.options.map((o) => String(o.value)).join("|"));
watch(signature, hide);
watch(() => props.disabled, (value) => value && hide());
</script>

<template>
  <div class="dd" ref="root" :class="{ open, disabled }">
    <button
      type="button"
      class="dd-face"
      :disabled="disabled"
      :aria-expanded="open"
      aria-haspopup="listbox"
      @click.stop.prevent="open ? hide() : show()"
      @keydown="onKey"
    >
      <span>{{ current?.short ?? current?.label }}</span>
      <i aria-hidden="true" />
    </button>
    <ul v-if="open" class="dd-list" role="listbox" @keydown="onKey">
      <li
        v-for="(option, index) in options"
        :key="String(option.value)"
        role="option"
        :aria-selected="option.value === modelValue"
        :class="{
          on: option.value === modelValue,
          cursor: index === active,
        }"
        @mouseenter="active = index"
        @click.stop.prevent="take(option)"
      >
        <span class="dd-tick" aria-hidden="true">{{ option.value === modelValue ? "→" : "" }}</span>
        {{ option.label }}
      </li>
    </ul>
  </div>
</template>
