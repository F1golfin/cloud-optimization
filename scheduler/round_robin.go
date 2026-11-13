
package main

import (
    "encoding/csv"
    "fmt"
    "os"
)

func readCSV(path string) ([][]string, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    return csv.NewReader(file).ReadAll()
}

func main() {
    if len(os.Args) != 4 {
        fmt.Println("Usage: round_robin <tasks.csv> <machines.csv> <output.csv>")
        os.Exit(1)
    }

    tasks, err := readCSV(os.Args[1])
    if err != nil {
        panic(err)
    }

    machines, err := readCSV(os.Args[2])
    if err != nil {
        panic(err)
    }

    // Remove CSV headers
    if len(tasks) < 2 || len(machines) < 2 {
        fmt.Fprintln(os.Stderr, "Error: missing tasks or machines")
        os.Exit(1)
    }
    tasks = tasks[1:]
    machines = machines[1:]

    output, err := os.Create(os.Args[3])
    if err != nil {
        panic(err)
    }
    defer output.Close()

    writer := csv.NewWriter(output)
    defer writer.Flush()

    writer.Write([]string{"task_id", "workload", "size", "vm"})
    seenTasks := make(map[string]bool)

    // Validate all task IDs before creating assignments
    for _, task := range tasks {
       if len(task) != 3 || task[0] == "" ||
           task[1] == "" || task[2] == "" {
           fmt.Fprintln(os.Stderr, "Error: invalid task")
           os.Exit(1)
       }

       if seenTasks[task[0]] {
           fmt.Fprintln(os.Stderr,
               "Error: duplicate task ID:", task[0])
           os.Exit(1)
       }

       seenTasks[task[0]] = true
   }

   // Generate the Round-Robin allocation
   for i, task := range tasks {
       vm := machines[i%len(machines)][0]

        writer.Write([]string{
            task[0],
            task[1],
            task[2],
            vm,
        })

        fmt.Printf("%s -> %s\n", task[0], vm)
    }


    fmt.Println("Allocation generated:", os.Args[3])
}
