Label + control + hint/error wrapper. Every configuration control in the console is wrapped in a Field so the consequence can be stated in plain language.

```jsx
<Field label="max_sessions" htmlFor="ms" hint="Baisser ce quota ne coupe pas les binds vivants.">
  <Input id="ms" mono defaultValue="4" />
</Field>
```

`error` replaces `hint` when present.
