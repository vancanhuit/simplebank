export interface LoginInput {
  username: string;
  password: string;
}

export interface RegistrationInput extends LoginInput {
  fullName: string;
  email: string;
}

export interface ValidationResult<T> {
  values: T;
  errors: Partial<Record<keyof T, string>>;
}

const ALPHANUMERIC = /^[A-Za-z0-9]+$/;
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const utf8 = new TextEncoder();

export function validateLogin(input: LoginInput): ValidationResult<LoginInput> {
  const values = {
    username: input.username.trim(),
    password: input.password,
  };
  const errors: ValidationResult<LoginInput>["errors"] = {};

  if (!values.username) {
    errors.username = "Enter your username.";
  } else if (!ALPHANUMERIC.test(values.username)) {
    errors.username = "Use letters and numbers only.";
  }

  if (!values.password) {
    errors.password = "Enter your password.";
  }

  return { values, errors };
}

export function validateRegistration(
  input: RegistrationInput,
): ValidationResult<RegistrationInput> {
  const values = {
    fullName: input.fullName.trim(),
    username: input.username.trim(),
    email: input.email.trim(),
    password: input.password,
  };
  const errors: ValidationResult<RegistrationInput>["errors"] = {};

  if (!values.fullName) {
    errors.fullName = "Enter your full name.";
  }

  if (!values.username) {
    errors.username = "Enter your username.";
  } else if (!ALPHANUMERIC.test(values.username)) {
    errors.username = "Use letters and numbers only.";
  }

  if (!EMAIL.test(values.email)) {
    errors.email = "Enter a valid email address.";
  }

  if (!values.password) {
    errors.password = "Enter your password.";
  } else if ([...values.password].length < 15) {
    errors.password = "Use at least 15 characters.";
  } else if (utf8.encode(values.password).byteLength > 72) {
    errors.password = "Use no more than 72 bytes.";
  }

  return { values, errors };
}
