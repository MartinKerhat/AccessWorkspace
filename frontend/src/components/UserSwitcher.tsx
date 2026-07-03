import type { User } from "../types";

type Props = {
  value: string;
  users: User[];
  onChange: (next: string) => void;
};

export function UserSwitcher({ value, users, onChange }: Props) {
  return (
    <label className="user-switcher">
      <span>Dev user</span>
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        {users.map((user) => (
          <option key={user.id} value={user.id}>
            {user.name} ({user.id})
          </option>
        ))}
      </select>
    </label>
  );
}
