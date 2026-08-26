import axios from "axios";

export const api = axios.create({
  baseURL: "/api/v1",
  timeout: 5_000,
  headers: {
    Accept: "application/json",
  },
});
